/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package omni

import (
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"io"
	"math/big"
	"slices"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/hkdf"
)

const (
	loginDomainSeparationTag = "Spanner-Omni-Login"
	authKeyInfo              = "AuthKey"
	exportKeyInfo            = "ExportKey"
	privateKeyInfo           = "PrivateKey"
	maskingKeyInfo           = "MaskingKey"
	diffieHellmanKeyInfo     = "OPAQUE-DeriveDiffieHellmanKeyPair"
)

// userAuthenticator manages the client state for OPAQUE login authentication.
type userAuthenticator struct {
	username              string
	password              []byte
	blind                 []byte
	clientPublicKeyshare  []byte
	clientPrivateKeyshare []byte
	clientNonce           []byte
	argon2Params          *HashParameters_Argon2IdParameters
}

// newAuthenticator creates a new userAuthenticator instance configured with the server's HashParameters.
func newAuthenticator(username string, password []byte, hashParams *HashParameters) (*userAuthenticator, error) {
	if username == "" {
		return nil, fmt.Errorf("username cannot be empty")
	}
	if len(password) == 0 {
		return nil, fmt.Errorf("password cannot be empty")
	}
	if hashParams == nil {
		return nil, fmt.Errorf("hashParams cannot be nil")
	}
	argon2Params, ok := hashParams.GetParameters().(*HashParameters_Argon2IdParameters_)
	if !ok || argon2Params.Argon2IdParameters == nil {
		return nil, fmt.Errorf("expected non-nil Argon2IdParameters in HashParameters")
	}
	p := argon2Params.Argon2IdParameters
	if p.IterationCount < 1 || p.IterationCount > 10 {
		return nil, fmt.Errorf("invalid Argon2Id iteration count: %d (must be between 1 and 10)", p.IterationCount)
	}
	if p.MemoryUsage < 8 || p.MemoryUsage > 64*1024 {
		return nil, fmt.Errorf("invalid Argon2Id memory usage: %d (must be between 8 and 65536 KB)", p.MemoryUsage)
	}
	if p.Parallelism < 1 || p.Parallelism > 255 {
		return nil, fmt.Errorf("invalid Argon2Id parallelism: %d (must be between 1 and 255)", p.Parallelism)
	}
	if p.HashSize < 1 || p.HashSize > 512 {
		return nil, fmt.Errorf("invalid Argon2Id hash size: %d (must be between 1 and 512)", p.HashSize)
	}
	return &userAuthenticator{
		username:     username,
		password:     slices.Clone(password),
		argon2Params: p,
	}, nil
}

// InitialRequest generates the first message in the OPAQUE protocol handshake containing the blinded password.
func (ua *userAuthenticator) InitialRequest() (*LoginRequest, error) {
	if ua.password == nil {
		return nil, fmt.Errorf("authenticator already used or password not set")
	}
	defer func() {
		clear(ua.password)
		ua.password = nil
	}()
	blindedMessage, blind, err := blind(ua.password)
	if err != nil {
		return nil, err
	}
	ua.blind = blind
	clientNonce, err := nonce()
	if err != nil {
		return nil, err
	}
	ua.clientNonce = clientNonce
	randomNonce, err := nonce()
	if err != nil {
		return nil, err
	}
	publicKey, privateKey, err := deriveKeyPair(randomNonce, []byte(diffieHellmanKeyInfo))
	if err != nil {
		return nil, err
	}
	ua.clientPublicKeyshare = publicKey
	ua.clientPrivateKeyshare = privateKey

	return &LoginRequest{
		Username: ua.username,
		Request: &LoginRequest_OpaqueRequest{
			OpaqueRequest: &OpaqueLoginRequest{
				Request: &OpaqueLoginRequest_InitialRequest{
					InitialRequest: &InitialOpaqueLoginRequest{
						BlindedMessage:       blindedMessage,
						ClientNonce:          ua.clientNonce,
						ClientPublicKeyshare: ua.clientPublicKeyshare,
					},
				},
			},
		},
	}, nil
}

// FinalRequest processes the server's initial response and generates the client's final OPAQUE verification message.
func (ua *userAuthenticator) FinalRequest(initialResp *LoginResponse) (*LoginRequest, error) {
	if initialResp == nil {
		return nil, fmt.Errorf("initialResp cannot be nil")
	}
	if len(ua.clientPublicKeyshare) == 0 || len(ua.clientNonce) == 0 || len(ua.clientPrivateKeyshare) == 0 {
		return nil, fmt.Errorf("authenticator not initialized; InitialRequest must be called first")
	}
	defer func() {
		if ua.blind != nil {
			clear(ua.blind)
			ua.blind = nil
		}
		if ua.clientPrivateKeyshare != nil {
			clear(ua.clientPrivateKeyshare)
			ua.clientPrivateKeyshare = nil
		}
	}()
	opaqueResp := initialResp.GetOpaqueResponse().GetInitialResponse()
	if opaqueResp == nil {
		return nil, fmt.Errorf("expected initial opaque response")
	}
	exportKey, clientMac, err := ua.generateKe3(
		opaqueResp.EvaluatedMessage,
		opaqueResp.MaskingNonce,
		opaqueResp.MaskedResponse,
		opaqueResp.ServerNonce,
		opaqueResp.ServerMac,
		opaqueResp.ServerPublicKeyshare,
	)
	if err != nil {
		return nil, err
	}
	if exportKey != nil {
		clear(exportKey)
	}
	return &LoginRequest{
		Username: ua.username,
		Request: &LoginRequest_OpaqueRequest{
			OpaqueRequest: &OpaqueLoginRequest{
				Request: &OpaqueLoginRequest_FinalRequest{
					FinalRequest: &FinalOpaqueLoginRequest{
						ClientMac: clientMac,
					},
				},
			},
		},
	}, nil
}

func (ua *userAuthenticator) generateKe3(evaluatedElement, maskingNonce, maskedResponse, serverNonce, serverMac, serverPublicKeyshare []byte) (exportKey, clientMac []byte, err error) {
	var oprf, stretchedOprf, randomizedPassword, maskingKey, clientPrivateKey []byte
	var dh1, dh2, dh3, inputKeyMaterial, km2, km3, sessionKey []byte
	var recoveredExportKey []byte
	defer func() {
		clear(oprf)
		clear(stretchedOprf)
		clear(randomizedPassword)
		clear(maskingKey)
		clear(clientPrivateKey)
		clear(dh1)
		clear(dh2)
		clear(dh3)
		clear(inputKeyMaterial)
		clear(km2)
		clear(km3)
		clear(sessionKey)
		if err != nil {
			clear(recoveredExportKey)
		}
	}()

	oprf, err = finalize(ua.blind, evaluatedElement)
	if err != nil {
		return nil, nil, err
	}
	stretchedOprf, err = stretch(oprf, ua.argon2Params)
	if err != nil {
		return nil, nil, err
	}
	randomizedPassword, err = extract(slices.Concat(oprf, stretchedOprf))
	if err != nil {
		return nil, nil, err
	}
	maskingKey, err = expand(randomizedPassword, []byte(maskingKeyInfo), sha256.Size)
	if err != nil {
		return nil, nil, err
	}
	credentialResponsePad, err := expand(maskingKey, slices.Concat(maskingNonce, []byte("CredentialResponsePad")), len(maskedResponse))
	if err != nil {
		return nil, nil, err
	}
	serializedEnvelope, err := xorBytes(maskedResponse, credentialResponsePad)
	if err != nil {
		return nil, nil, err
	}
	publicKeyLength := len(ua.clientPublicKeyshare)
	nonceLength := len(ua.clientNonce)
	expectedEnvelopeLen := publicKeyLength + nonceLength + sha256.Size
	if len(serializedEnvelope) != expectedEnvelopeLen {
		return nil, nil, fmt.Errorf("invalid serialized envelope length: got %d, want %d", len(serializedEnvelope), expectedEnvelopeLen)
	}
	serverPublicKey := serializedEnvelope[:publicKeyLength]
	envelopeNonce := serializedEnvelope[publicKeyLength : publicKeyLength+nonceLength]
	authTag := serializedEnvelope[publicKeyLength+nonceLength:]

	recoveredExportKey, clientPrivateKey, err = recoverClient(ua.username, randomizedPassword, envelopeNonce, authTag, serverPublicKey)
	if err != nil {
		return nil, nil, err
	}
	dh1, err = diffieHellman(ua.clientPrivateKeyshare, serverPublicKeyshare)
	if err != nil {
		return nil, nil, err
	}
	dh2, err = diffieHellman(ua.clientPrivateKeyshare, serverPublicKey)
	if err != nil {
		return nil, nil, err
	}
	dh3, err = diffieHellman(clientPrivateKey, serverPublicKeyshare)
	if err != nil {
		return nil, nil, err
	}
	inputKeyMaterial = slices.Concat(dh1, dh2, dh3)
	preamble := ua.preamble(evaluatedElement, serverPublicKey, serverNonce, serverPublicKeyshare)
	km2, km3, sessionKey, err = deriveSharedKeys(inputKeyMaterial, preamble)
	if err != nil {
		return nil, nil, err
	}
	hashedPreamble := sha256Hash(preamble)
	expectedServerMac := mac(km2, hashedPreamble[:])
	if len(serverMac) != len(expectedServerMac) || subtle.ConstantTimeCompare(expectedServerMac, serverMac) != 1 {
		return nil, nil, fmt.Errorf("server mac mismatch")
	}
	clientMac = mac(km3, sha256Hash(slices.Concat(preamble, expectedServerMac)))
	return recoveredExportKey, clientMac, nil
}

func deriveSharedKeys(inputKeyMaterial, preamble []byte) (km2, km3, sessionKey []byte, err error) {
	var prk, handshakeSecret []byte
	defer func() {
		clear(prk)
		clear(handshakeSecret)
		if err != nil {
			clear(km2)
			clear(km3)
			clear(sessionKey)
		}
	}()

	prk, err = extract(inputKeyMaterial)
	if err != nil {
		return nil, nil, nil, err
	}
	preambleHash := sha256Hash(preamble)
	handshakeSecret, err = deriveSecret(prk, []byte("HandshakeSecret"), preambleHash)
	if err != nil {
		return nil, nil, nil, err
	}
	sessionKey, err = deriveSecret(prk, []byte("SessionKey"), preambleHash)
	if err != nil {
		return nil, nil, nil, err
	}
	km2, err = deriveSecret(handshakeSecret, []byte("ServerMAC"), []byte(""))
	if err != nil {
		return nil, nil, nil, err
	}
	km3, err = deriveSecret(handshakeSecret, []byte("ClientMAC"), []byte(""))
	if err != nil {
		return nil, nil, nil, err
	}
	return km2, km3, sessionKey, nil
}

func deriveSecret(inputKeyMaterial, label, transcriptHash []byte) ([]byte, error) {
	info := slices.Concat([]byte("OPAQUE-"), label, transcriptHash)
	return expand(inputKeyMaterial, info, sha256.Size)
}

func (ua *userAuthenticator) preamble(evaluatedElement, serverPublicKey, serverNonce, serverPublicKeyshare []byte) []byte {
	return slices.Concat([]byte("OPAQUEv1-"), []byte(ua.username), ua.clientNonce, ua.clientPublicKeyshare, serverPublicKey, evaluatedElement, serverNonce, serverPublicKeyshare)
}

// diffieHellman computes the Diffie-Hellman shared secret.
func diffieHellman(privateKey, publicKey []byte) ([]byte, error) {
	curve := elliptic.P256()
	x, y := elliptic.UnmarshalCompressed(curve, publicKey)
	if x == nil {
		return nil, fmt.Errorf("invalid public key")
	}
	sx, sy := curve.ScalarMult(x, y, privateKey)
	if sx == nil {
		return nil, fmt.Errorf("scalar multiplication failed")
	}
	return elliptic.MarshalCompressed(curve, sx, sy), nil
}

// recoverClient recovers the client's export key and private key from the envelope.
func recoverClient(username string, randomizedPassword, envelopeNonce, authTag, serverPublicKey []byte) (exportKey, clientPrivateKey []byte, err error) {
	var authKey, seed []byte
	var derivedExportKey, derivedClientPrivateKey []byte
	defer func() {
		clear(authKey)
		clear(seed)
		if err != nil {
			clear(derivedExportKey)
			clear(derivedClientPrivateKey)
		}
	}()

	authKey, err = expand(randomizedPassword, slices.Concat(envelopeNonce, []byte(authKeyInfo)), sha256.Size)
	if err != nil {
		return nil, nil, err
	}
	derivedExportKey, err = expand(randomizedPassword, slices.Concat(envelopeNonce, []byte(exportKeyInfo)), sha256.Size)
	if err != nil {
		return nil, nil, err
	}
	seed, err = expand(randomizedPassword, slices.Concat(envelopeNonce, []byte(privateKeyInfo)), sha256.Size)
	if err != nil {
		return nil, nil, err
	}
	_, derivedClientPrivateKey, err = deriveKeyPair(seed, []byte(diffieHellmanKeyInfo))
	if err != nil {
		return nil, nil, err
	}
	expectedTag := mac(authKey, slices.Concat(envelopeNonce, serverPublicKey, []byte(username)))
	if len(authTag) != len(expectedTag) || subtle.ConstantTimeCompare(expectedTag, authTag) != 1 {
		return nil, nil, fmt.Errorf("auth tag mismatch")
	}
	return derivedExportKey, derivedClientPrivateKey, nil
}

// finalize computes the OPRF output.
func finalize(blind []byte, evaluatedMessage []byte) ([]byte, error) {
	if len(blind) == 0 {
		return nil, fmt.Errorf("blind scalar cannot be empty")
	}
	curve := elliptic.P256()
	x, y := elliptic.UnmarshalCompressed(curve, evaluatedMessage)
	if x == nil {
		return nil, fmt.Errorf("invalid evaluated message point")
	}
	privateKey := new(big.Int).SetBytes(blind)
	order := curve.Params().N
	orderMinusTwo := new(big.Int).Sub(order, big.NewInt(2))
	inversedBlind := new(big.Int).Exp(privateKey, orderMinusTwo, order)
	bytesInversedBlind := make([]byte, 32)
	inversedBlind.FillBytes(bytesInversedBlind)
	defer clear(bytesInversedBlind)

	sx, sy := curve.ScalarMult(x, y, bytesInversedBlind)
	if sx == nil {
		return nil, fmt.Errorf("scalar multiplication failed")
	}
	return elliptic.MarshalCompressed(curve, sx, sy), nil
}

// deriveKeyPair derives a public/private keypair from a seed.
func deriveKeyPair(seed, info []byte) (publicKey, privateKey []byte, err error) {
	deriveInput := slices.Concat(seed, info)
	curve := elliptic.P256()
	order := curve.Params().N
	privateKey, err = randomOracleSha256(deriveInput, order)
	if err != nil {
		return nil, nil, err
	}
	pubX, pubY := curve.ScalarBaseMult(privateKey)
	if pubX == nil {
		clear(privateKey)
		return nil, nil, fmt.Errorf("scalar base mult failed")
	}
	return elliptic.MarshalCompressed(curve, pubX, pubY), privateKey, nil
}

// randomOracleSha256 implements a random oracle based on SHA-256.
func randomOracleSha256(x []byte, max *big.Int) ([]byte, error) {
	hashOutputLength := 256
	outputBitLength := max.BitLen() + hashOutputLength
	iterCount := (outputBitLength + hashOutputLength - 1) / hashOutputLength
	if iterCount*hashOutputLength > 130048 {
		return nil, fmt.Errorf("the domain bit length must not be greater than 130048. Desired bit length: %d", outputBitLength)
	}
	excessBitCount := uint((iterCount * hashOutputLength) - outputBitLength)
	hashOutput := big.NewInt(0)
	for i := 1; i <= iterCount; i++ {
		hashOutput = hashOutput.Lsh(hashOutput, uint(hashOutputLength))
		bignumBytes := slices.Concat([]byte{byte(i)}, x)
		hashedString := sha256Hash(bignumBytes)
		newBigNum := new(big.Int).SetBytes(hashedString)
		hashOutput = hashOutput.Add(hashOutput, newBigNum)
	}
	hashOutput = hashOutput.Rsh(hashOutput, excessBitCount)
	hashOutput = hashOutput.Mod(hashOutput, max)
	scalarLen := hashOutputLength / 8
	if maxLen := (max.BitLen() + 7) / 8; maxLen > scalarLen {
		scalarLen = maxLen
	}
	scalarBytes := make([]byte, scalarLen)
	hashOutput.FillBytes(scalarBytes)
	return scalarBytes, nil
}

// blind blinds the client password point using a random scalar.
func blind(plaintext []byte) (publicKey, privateKey []byte, err error) {
	hx, hy := hashToCurveP256(plaintext, []byte(loginDomainSeparationTag))
	curve := elliptic.P256()
	order := curve.Params().N
	var scalarInt *big.Int
	for {
		scalarInt, err = rand.Int(rand.Reader, order)
		if err != nil {
			return nil, nil, err
		}
		if scalarInt.Sign() != 0 {
			break
		}
	}
	scalar := make([]byte, 32)
	scalarInt.FillBytes(scalar)
	px, py := curve.ScalarMult(hx, hy, scalar)
	if px == nil {
		clear(scalar)
		return nil, nil, fmt.Errorf("scalar mult failed")
	}
	return elliptic.MarshalCompressed(curve, px, py), scalar, nil
}

// stretch stretches the OPRF output using Argon2Id.
func stretch(input []byte, params *HashParameters_Argon2IdParameters) ([]byte, error) {
	salt, err := expand(input, []byte("Stretch"), int(params.HashSize))
	if err != nil {
		return nil, err
	}
	return argon2Hash(input, salt, params.HashSize, params), nil
}

// expand expands key material using HKDF.
func expand(inputKeyMaterial, info []byte, size int) ([]byte, error) {
	expanded := hkdf.New(sha256.New, inputKeyMaterial, []byte(""), info)
	result := make([]byte, size)
	if _, err := io.ReadFull(expanded, result); err != nil {
		return nil, err
	}
	return result, nil
}

// extract extracts key material using HKDF.
func extract(inputKeyMaterial []byte) ([]byte, error) {
	return expand(inputKeyMaterial, []byte("Extract"), sha256.Size)
}

// argon2Hash hashes key material using Argon2Id.
func argon2Hash(input, salt []byte, outputLength uint32, params *HashParameters_Argon2IdParameters) []byte {
	return argon2.IDKey(input, salt, params.IterationCount, params.MemoryUsage, uint8(params.Parallelism), outputLength)
}

// sha256Hash computes the SHA-256 hash.
func sha256Hash(input []byte) []byte {
	result := sha256.Sum256(input)
	return result[:]
}

// nonce generates a cryptographically secure random nonce.
func nonce() ([]byte, error) {
	nonce := make([]byte, sha256.Size)
	if _, err := rand.Reader.Read(nonce); err != nil {
		return nil, err
	}
	return nonce, nil
}

// mac computes HMAC-SHA-256.
func mac(key, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return m.Sum(nil)
}

// xorBytes computes the bitwise XOR of two byte slices.
func xorBytes(a, b []byte) ([]byte, error) {
	if len(a) != len(b) {
		return nil, fmt.Errorf("xorBytes: slices must be the same length")
	}
	if len(a) == 0 {
		return nil, fmt.Errorf("xorBytes: slices must not be empty")
	}
	result := make([]byte, len(a))
	subtle.XORBytes(result, a, b)
	return result, nil
}

var (
	p256P, _         = new(big.Int).SetString("ffffffff00000001000000000000000000000000ffffffffffffffffffffffff", 16)
	p256A            = modP(big.NewInt(-3))
	p256B, _         = new(big.Int).SetString("5ac635d8aa3a93e7b3ebbd55769886bc651d06b0cc53b0f63bce3c3e27d2604b", 16)
	p256Z            = modP(big.NewInt(-10))
	p256PMinus1Over2 = new(big.Int).Rsh(new(big.Int).Sub(p256P, big.NewInt(1)), 1)
	p256PPlus1Over4  = new(big.Int).Rsh(new(big.Int).Add(p256P, big.NewInt(1)), 2)
)

func modP(x *big.Int) *big.Int {
	res := new(big.Int).Mod(x, p256P)
	if res.Sign() < 0 {
		res.Add(res, p256P)
	}
	return res
}

// expandMessageXMD implements expand_message_xmd for SHA-256 (RFC 9380 Section 5.3.1).
func expandMessageXMD(msg, dst []byte, lenInBytes int) []byte {
	ell := (lenInBytes + 31) / 32
	if len(dst) > 255 {
		h := sha256.Sum256(slices.Concat([]byte("H2C-OVERSIZE-DST-"), dst))
		dst = h[:]
	}
	dstLen := byte(len(dst))
	bIn := slices.Concat(
		make([]byte, 64),
		msg,
		[]byte{byte(lenInBytes >> 8), byte(lenInBytes)},
		[]byte{0x00},
		dst,
		[]byte{dstLen},
	)
	b0 := sha256.Sum256(bIn)

	b1Input := slices.Concat(b0[:], []byte{0x01}, dst, []byte{dstLen})
	b1 := sha256.Sum256(b1Input)

	res := make([]byte, 0, lenInBytes)
	res = append(res, b1[:]...)
	prev := b1

	for i := 2; i <= ell; i++ {
		tmp := make([]byte, 32)
		for j := 0; j < 32; j++ {
			tmp[j] = b0[j] ^ prev[j]
		}
		biInput := slices.Concat(tmp, []byte{byte(i)}, dst, []byte{dstLen})
		bi := sha256.Sum256(biInput)
		res = append(res, bi[:]...)
		prev = bi
	}
	return res[:lenInBytes]
}

// mapToCurveSSWU implements Simplified SWU mapping for P-256 (RFC 9380 Section 6.6.2).
func mapToCurveSSWU(u *big.Int) (x, y *big.Int) {
	tv1 := modP(new(big.Int).Mul(u, u))
	tv1 = modP(new(big.Int).Mul(p256Z, tv1))
	tv2 := modP(new(big.Int).Mul(tv1, tv1))
	tv2 = modP(new(big.Int).Add(tv2, tv1))
	tv3 := modP(new(big.Int).Add(tv2, big.NewInt(1)))
	tv3 = modP(new(big.Int).Mul(p256B, tv3))

	var tv4 *big.Int
	if tv2.Sign() != 0 {
		tv4 = modP(new(big.Int).Neg(tv2))
	} else {
		tv4 = new(big.Int).Set(p256Z)
	}
	tv4 = modP(new(big.Int).Mul(p256A, tv4))

	tv4Inv := new(big.Int).ModInverse(tv4, p256P)
	x1 := modP(new(big.Int).Mul(tv3, tv4Inv))

	x1Cube := modP(new(big.Int).Exp(x1, big.NewInt(3), p256P))
	ax1 := modP(new(big.Int).Mul(p256A, x1))
	gx1 := modP(new(big.Int).Add(new(big.Int).Add(x1Cube, ax1), p256B))

	e1 := new(big.Int).Exp(gx1, p256PMinus1Over2, p256P)
	isSquare := (e1.Cmp(big.NewInt(1)) == 0 || gx1.Sign() == 0)

	if isSquare {
		x = x1
		y = new(big.Int).Exp(gx1, p256PPlus1Over4, p256P)
	} else {
		x = modP(new(big.Int).Mul(tv1, x1))
		x2Cube := modP(new(big.Int).Exp(x, big.NewInt(3), p256P))
		ax2 := modP(new(big.Int).Mul(p256A, x))
		gx2 := modP(new(big.Int).Add(new(big.Int).Add(x2Cube, ax2), p256B))
		y = new(big.Int).Exp(gx2, p256PPlus1Over4, p256P)
	}

	if u.Bit(0) != y.Bit(0) {
		y = modP(new(big.Int).Neg(y))
	}
	return x, y
}

// hashToCurveP256 implements P256_XMD:SHA-256_SSWU_RO_ (RFC 9380 Section 8.2).
func hashToCurveP256(msg, dst []byte) (x, y *big.Int) {
	uniformBytes := expandMessageXMD(msg, dst, 96)
	u0 := modP(new(big.Int).SetBytes(uniformBytes[:48]))
	u1 := modP(new(big.Int).SetBytes(uniformBytes[48:]))

	x0, y0 := mapToCurveSSWU(u0)
	x1, y1 := mapToCurveSSWU(u1)

	curve := elliptic.P256()
	if !curve.IsOnCurve(x0, y0) {
		fmt.Printf("x0,y0 NOT ON CURVE: x0=%x y0=%x\n", x0, y0)
	}
	if !curve.IsOnCurve(x1, y1) {
		fmt.Printf("x1,y1 NOT ON CURVE: x1=%x y1=%x\n", x1, y1)
	}
	return curve.Add(x0, y0, x1, y1)
}
