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

	"filippo.io/nistec"
	"github.com/bytemare/hash2curve/nist/p256"
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
	if hashParams == nil {
		return nil, fmt.Errorf("hashParams cannot be nil")
	}
	argon2Params, ok := hashParams.GetParameters().(*HashParameters_Argon2IdParameters_)
	if !ok || argon2Params.Argon2IdParameters == nil {
		return nil, fmt.Errorf("expected non-nil Argon2IdParameters in HashParameters")
	}
	p := argon2Params.Argon2IdParameters
	if p.IterationCount < 1 {
		return nil, fmt.Errorf("invalid Argon2Id iteration count: %d", p.IterationCount)
	}
	if p.MemoryUsage < 8 {
		return nil, fmt.Errorf("invalid Argon2Id memory usage: %d", p.MemoryUsage)
	}
	if p.Parallelism < 1 || p.Parallelism > 255 {
		return nil, fmt.Errorf("invalid Argon2Id parallelism: %d (must be between 1 and 255)", p.Parallelism)
	}
	if p.HashSize < 1 {
		return nil, fmt.Errorf("invalid Argon2Id hash size: %d", p.HashSize)
	}
	return &userAuthenticator{
		username:     username,
		password:     slices.Clone(password),
		argon2Params: p,
	}, nil
}

// InitialRequest generates the first message in the OPAQUE protocol handshake containing the blinded password.
func (ua *userAuthenticator) InitialRequest() (*LoginRequest, error) {
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
	opaqueResp := initialResp.GetOpaqueResponse().GetInitialResponse()
	if opaqueResp == nil {
		return nil, fmt.Errorf("expected initial opaque response")
	}
	_, clientMac, err := ua.generateKe3(
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
	oprf, err := finalize(ua.blind, evaluatedElement)
	if err != nil {
		return nil, nil, err
	}
	stretchedOprf, err := stretch(oprf, ua.argon2Params)
	if err != nil {
		return nil, nil, err
	}
	randomizedPassword, err := extract(slices.Concat(oprf, stretchedOprf))
	if err != nil {
		return nil, nil, err
	}
	maskingKey, err := expand(randomizedPassword, []byte(maskingKeyInfo), sha256.Size)
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
	if len(serializedEnvelope) < publicKeyLength+nonceLength {
		return nil, nil, fmt.Errorf("invalid serialized envelope length: got %d, want at least %d", len(serializedEnvelope), publicKeyLength+nonceLength)
	}
	serverPublicKey := serializedEnvelope[:publicKeyLength]
	envelopeNonce := serializedEnvelope[publicKeyLength : publicKeyLength+nonceLength]
	authTag := serializedEnvelope[publicKeyLength+nonceLength:]

	exportKey, clientPrivateKey, err := recoverClient(ua.username, randomizedPassword, envelopeNonce, authTag, serverPublicKey)
	if err != nil {
		return nil, nil, err
	}
	dh1, err := diffieHellman(ua.clientPrivateKeyshare, serverPublicKeyshare)
	if err != nil {
		return nil, nil, err
	}
	dh2, err := diffieHellman(ua.clientPrivateKeyshare, serverPublicKey)
	if err != nil {
		return nil, nil, err
	}
	dh3, err := diffieHellman(clientPrivateKey, serverPublicKeyshare)
	if err != nil {
		return nil, nil, err
	}
	inputKeyMaterial := slices.Concat(dh1, dh2, dh3)
	preamble := ua.preamble(evaluatedElement, serverPublicKey, serverNonce, serverPublicKeyshare)
	km2, km3, _, err := deriveSharedKeys(inputKeyMaterial, preamble)
	if err != nil {
		return nil, nil, err
	}
	hashedPreamble := sha256Hash(preamble)
	expectedServerMac := mac(km2, hashedPreamble[:])
	if len(serverMac) != len(expectedServerMac) || subtle.ConstantTimeCompare(expectedServerMac, serverMac) != 1 {
		return nil, nil, fmt.Errorf("server mac mismatch")
	}
	clientMac = mac(km3, sha256Hash(slices.Concat(preamble, expectedServerMac)))
	return exportKey, clientMac, nil
}

func deriveSharedKeys(inputKeyMaterial, preamble []byte) (km2, km3, sessionKey []byte, err error) {
	prk, err := extract(inputKeyMaterial)
	if err != nil {
		return nil, nil, nil, err
	}
	preambleHash := sha256Hash(preamble)
	handshakeSecret, err := deriveSecret(prk, []byte("HandshakeSecret"), preambleHash)
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
	point, err := nistec.NewP256Point().SetBytes(publicKey)
	if err != nil {
		return nil, err
	}
	secretPoint, err := point.ScalarMult(point, privateKey)
	if err != nil {
		return nil, err
	}
	return secretPoint.BytesCompressed(), nil
}

// recoverClient recovers the client's export key and private key from the envelope.
func recoverClient(username string, randomizedPassword, envelopeNonce, authTag, serverPublicKey []byte) (exportKey, clientPrivateKey []byte, err error) {
	authKey, err := expand(randomizedPassword, slices.Concat(envelopeNonce, []byte(authKeyInfo)), sha256.Size)
	if err != nil {
		return nil, nil, err
	}
	exportKey, err = expand(randomizedPassword, slices.Concat(envelopeNonce, []byte(exportKeyInfo)), sha256.Size)
	if err != nil {
		return nil, nil, err
	}
	seed, err := expand(randomizedPassword, slices.Concat(envelopeNonce, []byte(privateKeyInfo)), sha256.Size)
	if err != nil {
		return nil, nil, err
	}
	_, clientPrivateKey, err = deriveKeyPair(seed, []byte(diffieHellmanKeyInfo))
	if err != nil {
		return nil, nil, err
	}
	expectedTag := mac(authKey, slices.Concat(envelopeNonce, serverPublicKey, []byte(username)))
	if len(authTag) != len(expectedTag) || subtle.ConstantTimeCompare(expectedTag, authTag) != 1 {
		return nil, nil, fmt.Errorf("auth tag mismatch")
	}
	return exportKey, clientPrivateKey, nil
}

// finalize computes the OPRF output.
func finalize(blind []byte, evaluatedMessage []byte) ([]byte, error) {
	evaluatedElement, err := nistec.NewP256Point().SetBytes(evaluatedMessage)
	if err != nil {
		return nil, err
	}
	privateKey := new(big.Int).SetBytes(blind)
	curve := elliptic.P256()
	order := curve.Params().N
	inversedBlind := new(big.Int)
	if inversedBlind = inversedBlind.ModInverse(privateKey, order); inversedBlind == nil {
		return nil, fmt.Errorf("failed to compute modular inverse of blind")
	}
	bytesInversedBlind := make([]byte, 32)
	inversedBlind.FillBytes(bytesInversedBlind)
	oprf, err := evaluatedElement.ScalarMult(evaluatedElement, bytesInversedBlind)
	if err != nil {
		return nil, err
	}
	return oprf.BytesCompressed(), nil
}

// deriveKeyPair derives a public/private keypair from a seed.
func deriveKeyPair(seed, info []byte) (publicKey, privateKey []byte, err error) {
	p := nistec.NewP256Point()
	p = p.SetGenerator()
	deriveInput := slices.Concat(seed, info)
	curve := elliptic.P256()
	order := curve.Params().N
	privateKey, err = randomOracleSha256(deriveInput, order)
	if err != nil {
		return nil, nil, err
	}
	pubKey, err := p.ScalarMult(p, privateKey)
	if err != nil {
		return nil, nil, err
	}
	return pubKey.BytesCompressed(), privateKey, nil
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
		bignumBytes := slices.Concat(big.NewInt(int64(i)).Bytes(), x)
		hashedString := sha256Hash(bignumBytes)
		newBigNum := big.NewInt(0)
		newBigNum.SetBytes(hashedString)
		hashOutput = hashOutput.Add(hashOutput, newBigNum)
	}
	hashOutput = hashOutput.Rsh(hashOutput, excessBitCount)
	hashOutput = hashOutput.Mod(hashOutput, max)
	scalarBytes := make([]byte, hashOutputLength/8)
	hashOutput.FillBytes(scalarBytes)
	return scalarBytes, nil
}

// blind blinds the client password point using a random scalar.
func blind(plaintext []byte) (publicKey, privateKey []byte, err error) {
	point := p256.HashToCurve(plaintext, []byte(loginDomainSeparationTag))
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
	point, err = point.ScalarMult(point, scalar)
	if err != nil {
		return nil, nil, err
	}
	return point.BytesCompressed(), scalar, nil
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
