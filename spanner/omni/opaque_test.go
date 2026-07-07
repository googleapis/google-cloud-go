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
	"bytes"
	"fmt"
	"math/big"
	"testing"

	"filippo.io/nistec"
	"github.com/bytemare/hash2curve/nist/p256"
)

func TestRandomOracleSha256(t *testing.T) {
	max := new(big.Int).Lsh(big.NewInt(1), 63)
	tests := []struct {
		input      []byte
		iterations int
	}{
		{
			input:      []byte("key"),
			iterations: 1000,
		},
		{
			input:      []byte("key2"),
			iterations: 1000,
		},
		{
			input:      []byte{97, 97, 98, 99, 100, 101},
			iterations: 1000,
		},
	}
	for _, test := range tests {
		expectedOutput, err := randomOracleSha256(test.input, max)
		if err != nil {
			t.Fatalf("randomOracleSha256() failed: %v", err)
		}
		for i := 0; i < test.iterations; i++ {
			output, err := randomOracleSha256(test.input, max)
			if err != nil {
				t.Fatalf("randomOracleSha256() failed: %v", err)
			}
			if !bytes.Equal(output, expectedOutput) {
				t.Errorf("randomOracleSha256() = %v, want %v", output, expectedOutput)
			}
			if len(output) != 32 {
				t.Errorf("randomOracleSha256() length = %d, want 32", len(output))
			}
		}
	}
}

func TestMac(t *testing.T) {
	tests := []struct {
		key, data []byte
	}{
		{
			key:  []byte("key"),
			data: []byte("data"),
		},
		{
			key:  []byte("key"),
			data: []byte("data2"),
		},
		{
			key:  []byte{97, 97, 98, 99, 100, 101},
			data: []byte{102, 103, 104, 105, 106, 107},
		},
	}
	for _, test := range tests {
		mac1 := mac(test.key, test.data)
		mac2 := mac(test.key, test.data)
		if !bytes.Equal(mac1, mac2) {
			t.Errorf("mac() = %s, want %s", mac2, mac1)
		}
		if len(mac1) != macTagLength {
			t.Errorf("mac() length = %d, want %d", len(mac1), macTagLength)
		}
	}
}

func TestXorBytes(t *testing.T) {
	tests := []struct {
		input, mask []byte
		wantErr     bool
	}{
		{
			input: []byte("abc"),
			mask:  []byte("def"),
		},
		{
			input: []byte{97, 97, 98, 99, 100, 101},
			mask:  []byte{102, 103, 104, 105, 106, 107},
		},
		{
			input: []byte{97, 97, 98, 99, 100, 101},
			mask:  []byte{0, 0, 0, 0, 0, 0},
		},
		{
			input:   []byte("abc"),
			mask:    []byte("defghi"),
			wantErr: true,
		},
		{
			input:   []byte("abcdefghi"),
			mask:    []byte("jklmnop"),
			wantErr: true,
		},
		{
			input:   []byte{},
			mask:    []byte{},
			wantErr: true,
		},
	}
	for _, test := range tests {
		xored, err := xorBytes(test.input, test.mask)
		if test.wantErr {
			if err == nil {
				t.Errorf("xorBytes() expected error for input %v and mask %v", test.input, test.mask)
			}
			continue
		}
		if err != nil {
			t.Fatalf("xorBytes() failed: %v", err)
		}
		if len(xored) != len(test.input) {
			t.Errorf("xorBytes() length = %d, want %d", len(xored), len(test.input))
		}
		original, err := xorBytes(xored, test.mask)
		if err != nil {
			t.Fatalf("xorBytes() failed: %v", err)
		}
		if !bytes.Equal(original, test.input) {
			t.Errorf("xored bytes do not match expected bytes: %s, %s", original, test.input)
		}
	}
}

func TestOpfrEvaluate(t *testing.T) {
	username := "username"
	password := []byte("password1234")
	oprfSeed, err := nonce()
	if err != nil {
		t.Fatalf("nonce() failed: %v", err)
	}
	seed, err := expand(oprfSeed, []byte(username+"OprfKey"), 32)
	if err != nil {
		t.Fatalf("expand() failed: %v", err)
	}
	_, serverPrivateKey, err := deriveKeyPair(seed, []byte("OPAQUE-DeriveKeyPair"))
	if err != nil {
		t.Fatalf("deriveKeys() failed: %v", err)
	}
	blindedElement, blind, err := blind(password)
	if err != nil {
		t.Fatalf("blind() failed: %v", err)
	}
	evaluatedElement, err := blindEvaluate(t, username, blindedElement, oprfSeed)
	if err != nil {
		t.Fatalf("blindEvaluate() failed: %v", err)
	}
	oprf, err := finalize(blind, evaluatedElement)
	if err != nil {
		t.Fatalf("finalize() failed: %v", err)
	}
	prf, err := evaluate(t, password, serverPrivateKey)
	if err != nil {
		t.Fatalf("evaluate() failed: %v", err)
	}
	if !bytes.Equal(oprf, prf) {
		t.Errorf("oprf does not match prf: %s, %s", oprf, prf)
	}
}

func TestDeriveKeyPair(t *testing.T) {
	tests := []struct {
		seed1, info1, seed2, info2 []byte
		wantDifferentKeys          bool
	}{
		{
			seed1: []byte("seed"),
			info1: []byte("info"),
			seed2: []byte("seed"),
			info2: []byte("info"),
		},
		{
			seed1: []byte("seed2"),
			info1: []byte("info"),
			seed2: []byte("seed2"),
			info2: []byte("info"),
		},
		{
			seed1: []byte("seed"),
			info1: []byte("info2"),
			seed2: []byte("seed"),
			info2: []byte("info2"),
		},
		{
			seed1:             []byte("seed"),
			info1:             []byte("info2"),
			seed2:             []byte("different"),
			info2:             []byte("info2"),
			wantDifferentKeys: true,
		},
		{
			seed1:             []byte("seed"),
			info1:             []byte("info2"),
			seed2:             []byte("seed"),
			info2:             []byte("info1"),
			wantDifferentKeys: true,
		},
	}
	for _, test := range tests {
		publicKey, privateKey, err := deriveKeyPair(test.seed1, test.info1)
		if err != nil {
			t.Fatalf("deriveKeyPair() failed: %v", err)
		}
		publicKey2, privateKey2, err := deriveKeyPair(test.seed2, test.info2)
		if err != nil {
			t.Fatalf("deriveKeyPair() failed: %v", err)
		}
		if test.wantDifferentKeys {
			if bytes.Equal(privateKey, privateKey2) {
				t.Errorf("private key should be different: %s, %s", privateKey, privateKey2)
			}
			if bytes.Equal(publicKey, publicKey2) {
				t.Errorf("public key should be different: %s, %s", publicKey, publicKey2)
			}
		} else {
			if !bytes.Equal(privateKey, privateKey2) {
				t.Errorf("private key does not match recovered private key: %s, %s", privateKey, privateKey2)
			}
			if !bytes.Equal(publicKey, publicKey2) {
				t.Errorf("public key does not match recovered public key: %s, %s", publicKey, publicKey2)
			}
		}
	}
}

func TestStretch(t *testing.T) {
	params := &HashParameters_Argon2IdParameters{
		IterationCount: 5,
		MemoryUsage:    7 * 1024,
		Parallelism:    1,
		HashSize:       32,
	}
	longInput := make([]byte, 1024)
	for i := range longInput {
		longInput[i] = byte(i)
	}
	tests := []struct {
		input          []byte
		expectedOutput []byte
	}{
		{
			input:          []byte{},
			expectedOutput: []byte{58, 42, 135, 162, 54, 231, 153, 103, 111, 241, 220, 39, 245, 158, 231, 5, 157, 108, 133, 178, 37, 97, 185, 220, 104, 13, 66, 147, 221, 19, 198, 9},
		},
		{
			input:          []byte("input"),
			expectedOutput: []byte{177, 173, 204, 142, 245, 214, 91, 164, 139, 85, 150, 101, 204, 187, 48, 176, 251, 7, 154, 247, 251, 35, 241, 135, 99, 117, 14, 121, 182, 124, 87, 46},
		},
		{
			input:          []byte{97, 97, 98, 99, 100, 101},
			expectedOutput: []byte{164, 94, 8, 109, 17, 19, 42, 55, 86, 44, 54, 89, 255, 148, 130, 248, 133, 4, 40, 24, 246, 27, 81, 56, 231, 137, 238, 30, 67, 159, 3, 157},
		},
		{
			input:          longInput,
			expectedOutput: []byte{132, 52, 182, 135, 97, 18, 8, 254, 10, 1, 94, 98, 78, 193, 246, 160, 12, 209, 142, 253, 247, 115, 4, 149, 141, 2, 105, 159, 139, 94, 161, 116},
		},
	}
	for _, test := range tests {
		stretched, err := stretch(test.input, params)
		if err != nil {
			t.Fatalf("stretch() failed: %v", err)
		}
		if len(stretched) != 32 {
			t.Errorf("stretch() length = %d, want 32", len(stretched))
		}
		if !bytes.Equal(stretched, test.expectedOutput) {
			t.Errorf("stretch() = %v, want %v", stretched, test.expectedOutput)
		}
	}
}

func TestDiffieHellman(t *testing.T) {
	tests := []struct {
		serverSeed, clientSeed []byte
	}{
		{
			serverSeed: []byte{},
			clientSeed: []byte{},
		},
		{
			serverSeed: []byte("server-seed"),
			clientSeed: []byte("client-seed"),
		},
		{
			serverSeed: []byte("server-seed2"),
			clientSeed: []byte("client-seed2"),
		},
		{
			serverSeed: []byte("no-need-to-be-the-same-length"),
			clientSeed: []byte("im-a-shorter-seed"),
		},
	}
	for _, test := range tests {
		serverPublicKey, serverPrivateKey, err := deriveKeyPair(test.serverSeed, []byte(diffieHellmanKeyInfo))
		if err != nil {
			t.Fatalf("deriveKeyPair() failed: %v", err)
		}
		clientPublicKey, clientPrivateKey, err := deriveKeyPair(test.clientSeed, []byte(diffieHellmanKeyInfo))
		if err != nil {
			t.Fatalf("deriveKeyPair() failed: %v", err)
		}
		serverSharedSecret, err := diffieHellman(serverPrivateKey, clientPublicKey)
		if err != nil {
			t.Fatalf("diffieHellman() failed: %v", err)
		}
		clientSharedSecret, err := diffieHellman(clientPrivateKey, serverPublicKey)
		if err != nil {
			t.Fatalf("diffieHellman() failed: %v", err)
		}
		if !bytes.Equal(serverSharedSecret, clientSharedSecret) {
			t.Errorf("server and client secrets do not match: %v, %v", serverSharedSecret, clientSharedSecret)
		}
	}
}

func TestExtract(t *testing.T) {
	longInput := make([]byte, 1024)
	for i := range longInput {
		longInput[i] = byte(i)
	}
	tests := []struct {
		input          []byte
		expectedOutput []byte
	}{
		{
			input:          []byte{},
			expectedOutput: []byte{99, 252, 241, 111, 84, 209, 178, 181, 88, 96, 91, 194, 149, 79, 240, 143, 252, 68, 135, 177, 69, 144, 33, 115, 195, 224, 100, 31, 46, 160, 150, 41},
		},
		{
			input:          []byte("input"),
			expectedOutput: []byte{94, 113, 123, 114, 170, 250, 213, 241, 247, 203, 160, 141, 111, 233, 68, 240, 123, 33, 207, 139, 115, 44, 249, 217, 77, 34, 6, 254, 77, 75, 20, 99},
		},
		{
			input:          []byte{97, 97, 98, 99, 100, 101},
			expectedOutput: []byte{48, 112, 244, 9, 53, 2, 10, 147, 218, 132, 43, 198, 200, 101, 20, 3, 71, 158, 227, 3, 161, 15, 215, 112, 251, 195, 187, 96, 11, 203, 226, 210},
		},
		{
			input:          longInput,
			expectedOutput: []byte{246, 148, 220, 16, 96, 62, 53, 189, 96, 83, 146, 84, 233, 183, 89, 12, 235, 31, 24, 113, 148, 25, 213, 33, 167, 78, 147, 162, 223, 115, 38, 117},
		},
	}
	for _, test := range tests {
		extracted, err := extract(test.input)
		if err != nil {
			t.Fatalf("extract() failed: %v", err)
		}
		if len(extracted) != 32 {
			t.Errorf("extract() length = %d, want 32", len(extracted))
		}
		if !bytes.Equal(extracted, test.expectedOutput) {
			t.Errorf("extract() = %v, want %v", extracted, test.expectedOutput)
		}
	}
}

func TestNewAuthenticatorValidation(t *testing.T) {
	t.Run("nil hashParams", func(t *testing.T) {
		_, err := newAuthenticator("user", []byte("pass"), nil)
		if err == nil {
			t.Errorf("expected error for nil hashParams")
		}
	})

	t.Run("nil argon2Params", func(t *testing.T) {
		hashParams := &HashParameters{
			Parameters: &HashParameters_Argon2IdParameters_{
				Argon2IdParameters: nil,
			},
		}
		_, err := newAuthenticator("user", []byte("pass"), hashParams)
		if err == nil {
			t.Errorf("expected error for nil Argon2IdParameters")
		}
	})

	t.Run("invalid argon2Params fields", func(t *testing.T) {
		invalidCases := []struct {
			name   string
			params *HashParameters_Argon2IdParameters
		}{
			{
				name: "invalid IterationCount",
				params: &HashParameters_Argon2IdParameters{
					IterationCount: 0,
					MemoryUsage:    64 * 1024,
					Parallelism:    4,
					HashSize:       32,
				},
			},
			{
				name: "invalid MemoryUsage",
				params: &HashParameters_Argon2IdParameters{
					IterationCount: 3,
					MemoryUsage:    4,
					Parallelism:    4,
					HashSize:       32,
				},
			},
			{
				name: "invalid Parallelism",
				params: &HashParameters_Argon2IdParameters{
					IterationCount: 3,
					MemoryUsage:    64 * 1024,
					Parallelism:    0,
					HashSize:       32,
				},
			},
			{
				name: "invalid HashSize",
				params: &HashParameters_Argon2IdParameters{
					IterationCount: 3,
					MemoryUsage:    64 * 1024,
					Parallelism:    4,
					HashSize:       0,
				},
			},
		}
		for _, tc := range invalidCases {
			t.Run(tc.name, func(t *testing.T) {
				hashParams := &HashParameters{
					Parameters: &HashParameters_Argon2IdParameters_{
						Argon2IdParameters: tc.params,
					},
				}
				_, err := newAuthenticator("user", []byte("pass"), hashParams)
				if err == nil {
					t.Errorf("expected error for %s", tc.name)
				}
			})
		}
	})
}

func blindEvaluate(t *testing.T, username string, pubKey, oprfSeed []byte) ([]byte, error) {
	t.Helper()
	seed, err := expand(oprfSeed, []byte(username+"OprfKey"), 32)
	if err != nil {
		return nil, fmt.Errorf("expand() failed: %v", err)
	}
	_, oprfKey, err := deriveKeyPair(seed, []byte("OPAQUE-DeriveKeyPair"))
	if err != nil {
		return nil, fmt.Errorf("deriveKeyPair() failed: %v", err)
	}
	blindElement, err := nistec.NewP256Point().SetBytes(pubKey)
	if err != nil {
		return nil, fmt.Errorf("SetBytes() failed: %v", err)
	}
	point, err := blindElement.ScalarMult(blindElement, oprfKey)
	if err != nil {
		return nil, fmt.Errorf("ScalarMult() failed: %v", err)
	}
	return point.Bytes(), nil
}

func evaluate(t *testing.T, password []byte, serverPrivateKey []byte) ([]byte, error) {
	t.Helper()
	inputElement := p256.HashToCurve(password, []byte(loginDomainSeparationTag))
	point, err := inputElement.ScalarMult(inputElement, serverPrivateKey)
	if err != nil {
		return nil, err
	}
	return point.BytesCompressed(), nil
}
