// Package externalaccount provides support for creating workload identity
// federation and workforce identity federation token sources that can be used
// to access Google Cloud resources from external identity providers.
//
// # Workforce Identity Federation
//
// Workforce identity federation lets you use an external identity provider
// (IdP) to authenticate and authorize a workforce—a group of users, such as
// employees, partners, and contractors—using IAM, so that the users can access
// Google Cloud services. Workforce identity federation extends Google Cloud's
// identity capabilities to support syncless, attribute-based single sign on.
//
// With workforce identity federation, your workforce can access Google Cloud resources
// using an external identity provider (IdP) that supports OpenID Connect (OIDC) or
// SAML 2.0 such as Azure Active Directory (Azure AD), Active Directory Federation
// Services (AD FS), Okta, and others.
//
// Follow the detailed instructions on how to configure Workload Identity Federation
// in various platforms:
//
//   - [Amazon Web Services (AWS)](https://cloud.google.com/iam/docs/workload-identity-federation-with-other-clouds#aws)
//   - [Azure AD](https://cloud.google.com/iam/docs/workforce-sign-in-azure-ad)
//   - [Okta](https://cloud.google.com/iam/docs/workforce-sign-in-okta)
//   - [OIDC identity provider](https://cloud.google.com/iam/docs/configuring-workforce-identity-federation#oidc)
//   - [SAML 2.0 identity provider](https://cloud.google.com/iam/docs/configuring-workforce-identity-federation#saml)
//
// For workforce identity federation, the library can retrieve tokens in three ways:
// from a local file location (file-sourced credentials), from a server
// (URL-sourced credentials), or from a local executable (executable-sourced
// credentials).
// For file-sourced credentials, a background process needs to be continuously
// refreshing the file location with a new OIDC/SAML token prior to expiration.
// For tokens with one hour lifetimes, the token needs to be updated in the file
// every hour. The token can be stored directly as plain text or in JSON format.
// For URL-sourced credentials, a local server needs to host a GET endpoint to
// return the OIDC/SAML token. The response can be in plain text or JSON.
// Additional required request headers can also be specified.
// For executable-sourced credentials, an application needs to be available to
// output the OIDC/SAML token and other information in a JSON format.
// For more information on how these work (and how to implement
// executable-sourced credentials), please check out:
// https://cloud.google.com/iam/docs/workforce-obtaining-short-lived-credentials#generate_a_configuration_file_for_non-interactive_sign-in
//
// # Security considerations
//
// Note that this library does not perform any validation on the token_url,
// token_info_url, or service_account_impersonation_url fields of the credential
// configuration. It is not recommended to use a credential configuration that
// you did not generate with the gcloud CLI unless you verify that the URL
// fields point to a googleapis.com domain.
package externalaccount
