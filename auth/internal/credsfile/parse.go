// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package credsfile

import (
	"encoding/json"
)

// ParseServiceAccount parses bytes into a [ServiceAccountFile].
func ParseServiceAccount(b []byte) (*ServiceAccountFile, error) {
	var f *ServiceAccountFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return f, nil
}

// ParseClientCredentials parses bytes into a
// [credsfile.ClientCredentialsFile].
func ParseClientCredentials(b []byte) (*ClientCredentialsFile, error) {
	var f *ClientCredentialsFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return f, nil
}

// ParseUserCredentials parses bytes into a [UserCredentialsFile].
func ParseUserCredentials(b []byte) (*UserCredentialsFile, error) {
	var f *UserCredentialsFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return f, nil
}

// ParseExternalAccount parses bytes into a [ExternalAccountFile].
func ParseExternalAccount(b []byte) (*ExternalAccountFile, error) {
	var f *ExternalAccountFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return f, nil
}

// ParseExternalAccountAuthorizedUser parses bytes into a
// [ExternalAccountAuthorizedUserFile].
func ParseExternalAccountAuthorizedUser(b []byte) (*ExternalAccountAuthorizedUserFile, error) {
	var f *ExternalAccountAuthorizedUserFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return f, nil
}

// ParseImpersonatedServiceAccount parses bytes into a
// [ImpersonatedServiceAccountFile].
func ParseImpersonatedServiceAccount(b []byte) (*ImpersonatedServiceAccountFile, error) {
	var f *ImpersonatedServiceAccountFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return f, nil
}

// ParseGDCHServiceAccount parses bytes into a [GDCHServiceAccountFile].
func ParseGDCHServiceAccount(b []byte) (*GDCHServiceAccountFile, error) {
	var f *GDCHServiceAccountFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return f, nil
}

type fileTypeChecker struct {
	Type string `json:"type"`
}

// ParseFileType determines the [CredentialType] based on bytes provided.
// Only returns error for json.Unmarshal.
// Returns UnknownCredType if no match.
func ParseFileType(b []byte) (CredentialType, error) {
	var f fileTypeChecker
	if err := json.Unmarshal(b, &f); err != nil {
		return 0, err
	}
	return parseCredentialType(f.Type), nil
}

// parseCredentialType returns the associated filetype based on the parsed
// typeString provided.
func parseCredentialType(typeString string) CredentialType {
	switch typeString {
	case "service_account":
		return ServiceAccountKey
	case "authorized_user":
		return UserCredentialsKey
	case "impersonated_service_account":
		return ImpersonatedServiceAccountKey
	case "external_account":
		return ExternalAccountKey
	case "external_account_authorized_user":
		return ExternalAccountAuthorizedUserKey
	case "gdch_service_account":
		return GDCHServiceAccountKey
	default:
		return UnknownCredType
	}
}

// ParseCredentialTypeString returns the associated filetype string based
// on the parsed type code int provided.
func ParseCredentialTypeString(credType CredentialType) string {
	switch credType {
	case ServiceAccountKey:
		return "service_account"
	case UserCredentialsKey:
		return "authorized_user"
	case ImpersonatedServiceAccountKey:
		return "impersonated_service_account"
	case ExternalAccountKey:
		return "external_account"
	case ExternalAccountAuthorizedUserKey:
		return "external_account_authorized_user"
	case GDCHServiceAccountKey:
		return "gdch_service_account"
	default:
		return "unknown"
	}
}
