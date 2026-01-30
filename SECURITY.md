# Security & Compliance

This document covers our security measures, responsible disclosure policies, and recommendations for secure usage.

## 1. Overview of Security Practices

Midaz utilizes the Ory Open-source Stack, which is designed with robust security measures to protect both the software and its users. Our security infrastructure includes multiple components:

* **Identity Management and Authentication:** An identity and user management server that handles user registration, login, and user profile management.
* **Authorization and Access Control:** OAuth2 provider that handles authentication and authorization.
* **Token Issuance and Management:** Implementation of attribute-based access control (ABAC) and access control policies.
* **Identity and Access Proxy:** Identity and Access Proxy (IAP) that validates incoming requests.

These components ensure that Midaz maintains high security standards and protects against unauthorized access and other potential security threats.

To learn more about the Ory Open-source Stack, visit the [Ory Website](https://www.ory.sh/).

## 2. Recommendations for Secure Usage and Configuration

To ensure the security of your deployments, we recommend the following best practices:

* **Avoid Hardcoding Sensitive Information:** Hardcoding passwords, API keys, or other sensitive data in the source code can lead to security vulnerabilities. Use environment variables or secure secrets management tools like HashiCorp Vault.
* **Use Secrets Management:** Store sensitive configuration such as passwords and API keys using secrets management tools. This prevents sensitive data from being exposed in your configuration files or source code.
* **Regular Updates:** Keep your Midaz installation and its dependencies up-to-date to protect against vulnerabilities.
* **Secure Configuration:** Follow our configuration guidelines in the official documentation to set up Midaz securely.

## 3. Responsible Disclosure Policy

For transparency, any known securities improvements join in our **[Github Discussions](https://github.com/LerianStudio/midaz/v3/discussions)**. This allows our community to follow the progress and updates related to security patches and enhancements.

If you discover a security vulnerability within Midaz, please report it using our responsible disclosure policy. Do not disclose the issue publicly until we have had the opportunity to address it.

* **Initial Contact:** Researcher submits vulnerability via secure email.
* **Acknowledgment:** We acknowledge receipt within 24 hours.
* **Verification:** Our security team verifies the vulnerability.
* **Impact Assessment:** We determine the severity and potential impact.
* **Resolution:** We develop and deploy a fix.
* **Notification:** We inform the researcher of the resolution.
* **Public Disclosure:** We coordinate with the researcher to publicly disclose the vulnerability responsibly.

### Contact Information:

* **Security Email:** [security@lerian.studio](security@lerian.studio)
* **PGP Key:** Available for secure communications on our security page. We recommend [Mailvelope](https://mailvelope.com/en) to encrypt emails.

We review all reports within 24 hours and work with you to resolve the issue quickly and confidentially.