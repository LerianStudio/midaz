# Security & Compliance

Welcome to the Midaz Security Practices documentation. This document provides an overview of our security measures, policies for responsible disclosure, and recommendations for secure usage.

## 1. Overview of Security Practices

Midaz utilizes the Ory Open-source Stack, which is designed with robust security measures to protect both the software and its users. Our security infrastructure includes multiple components:

* **Identity Management and Authentication** : An identity and user management server that handles user registration, login, and user profile management using modern identity.
* **Authorization and Access Control** : OAuth2 provider, which handles authentication and authorization.
* **Token Issuance and Management** : Implementation of attribute-based access control (ABAC) and access control policies.
* **Identity and Access Proxy** : Identity and Access Proxy (IAP) that validates incoming requests.

These components ensure that Midaz maintains high security standards and protects against unauthorized access and other potential security threats.

If you have more interest about the Ory Open-source Stack, see more in [Ory Website](https://www.ory.sh/).

## 2. Recommendations for Secure Usage and Configuration

To ensure the security of your deployments, we recommend the following best practices:

* **Avoid Hardcoding Sensitive Information** : Hardcoding passwords, API keys, or other sensitive data directly in the source code can lead to security vulnerabilities if the codebase is exposed or accessed by unauthorized individuals. Use environment variables or secure secrets management tools like HashiCorp Vault to manage sensitive configurations securely.
* **Use Secrets Management** : Store sensitive configuration such as passwords and API keys using secrets management tools. This prevents sensitive data from being exposed in your configuration files or source code.
* **Regular Updates** : Keep your Midaz installation and its dependencies up-to-date to protect against vulnerabilities.
* **Secure Configuration** : Follow our configuration guidelines to set up Midaz securely, available in the official documentation.

## 3. Responsible Disclosure Policy

For transparency, any known securities improvements join in our **[Github Discussions](https://github.com/LerianStudio/midaz/v3/discussions)**. This allows our community to follow the progress and updates related to security patches and enhancements.

If you discover a security vulnerability within Midaz, please report it using our responsible disclosure policy. Do not disclose the issue publicly until we have had the opportunity to address it.

* **Initial Contact** : Researcher submits vulnerability via secure email.
* **Acknowledgment** : Your team acknowledges receipt within 24 hours.
* **Verification** : Your security team verifies the vulnerability.
* **Impact Assessment** : Determine the severity and potential impact of the vulnerability.
* **Resolution** : Develop and deploy a fix.
* **Notification** : Inform the researcher of the resolution.
* **Public Disclosure** : Coordinate with the researcher to publicly disclose the vulnerability responsibly.

### Contact Information:

* **Security Email** : [security@lerian.studio](security@lerian.studio)
* **PGP Key** : Available for secure communications on our security page - We strongly recommend [Mailvelope](https://mailvelope.com/en) tool to encrypt sender and receiver emails.

We aim to review all reports within 24 hours and will work with you to understand and resolve the issue quickly and confidentially.
