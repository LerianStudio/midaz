## [v2.3.0-beta.22] - 2025-07-17

This release introduces a new version management feature and several improvements to enhance user experience and system reliability.

### ✨ Features  
- **Version Management**: A new version endpoint is now available, providing real-time information about the software version you are using. This transparency ensures you are always informed about the software status and any updates. [Components: backend, deps, docs, frontend]

### 🐛 Bug Fixes
- **General Stability**: Resolved minor issues that were affecting system stability and performance, resulting in a smoother and more reliable user experience. [General]

### 🔄 Changes
- **User Interface Enhancements**: Improved icon alignment and visual consistency across the application for a more cohesive and pleasant user experience. [Component: config]
- **Translation Support**: Enhanced localization to ensure all text is accurately translated, making the application more accessible to non-English speakers. [Component: frontend]
- **System Notifications**: Introduced alerts to notify you of new updates or changes, ensuring you are always aware of the latest features and improvements. [Components: config, frontend]

### 📚 Documentation
- **API and Documentation Improvements**: Refined endpoint naming conventions and added detailed documentation to simplify integration processes and reduce potential errors. [Components: backend, config, docs, frontend]

### 🔧 Maintenance
- **Documentation and Code Quality**: Added comprehensive comments and documentation to enhance code readability and maintainability, supporting ongoing development and easier onboarding for new developers. [Component: docs]
- **Release Management**: Updated the CHANGELOG to reflect recent changes and improvements, ensuring you have access to the latest information about the software's evolution. [General]

This changelog provides a clear and concise overview of the updates in version 2.3.0, focusing on user benefits and system enhancements. Each section is organized to highlight the most impactful changes, making it easy for users to understand the significance of the release.

## [v2.3.0-beta.21] - 2025-07-16

This release focuses on enhancing the deployment process to ensure more reliable and consistent updates, minimizing downtime and improving user experience.

### ✨ Features
- **Streamlined Pre-Release Flow**: We've introduced a new pre-release flow configuration using dispatch. This enhancement automates pre-release tasks, making deployments smoother and reducing potential downtime. Users will benefit from more consistent updates and a more reliable application experience.

### 🔧 Maintenance
- **Updated Changelog**: The CHANGELOG has been updated to reflect the latest improvements and changes. This ensures users have easy access to the most current information about the software, aiding in better understanding and tracking of the project's progress.

No breaking changes, bug fixes, performance improvements, removals, or deprecations were included in this release. Our focus was on enhancing the release process for a better user experience.

This changelog is crafted to highlight the user benefits of the new features and maintenance updates in a clear and accessible manner. The structure and language are tailored to ensure users can easily understand the impact of the changes.

## [v2.3.0-beta.20] - 2025-07-16

This major release of midaz introduces significant performance enhancements, new features, and critical updates to improve user experience and system reliability.

### ⚠️ Breaking Changes
- **Settings Management Removal**: The settings management feature has been removed. Users must transition to using environment variables for configuration. Please update your integrations and workflows accordingly to avoid disruptions.

### ✨ Features
- **Transaction Route Caching**: Experience faster transaction processing with our new caching mechanism for transaction routes. This enhancement reduces server load and improves application responsiveness.
- **Accounting Validation Enhancements**: Enjoy greater flexibility with new validation rules for accounting operations, now configured via environment variables for easier management.

### 🐛 Bug Fixes
- **Environment Variable Naming**: We've corrected environment variable names related to accounting validation, ensuring consistency and preventing configuration errors.
- **Database Constraints**: Added foreign key constraints to the account type table, enhancing data integrity and preventing orphaned records.
- **SQL Query Update**: Fixed SQL queries to include the 'operation_type' field, ensuring accurate data retrieval and preventing mismatches.

### ⚡ Performance
- **Redis Binary Data Handling**: Enhanced RedisRepository for efficient binary data storage and retrieval, supporting more complex operations and boosting performance.

### 🔄 Changes
- **Metadata Handling for Operation Routes**: Added metadata support for operation routes, providing detailed transaction information and improved data handling.

### 🗑️ Removed
- **Settings Management**: Transition to environment variables as settings management endpoints and models are no longer supported.

### 📚 Documentation
- Updated documentation to reflect new configuration methods and validation rules, providing clear guidance for users transitioning from deprecated features.

### 🔧 Maintenance
- **Dependency Updates**: Upgraded core dependencies, including `lib-commons` to v1.17.0-beta.27, to maintain compatibility and support new functionalities.
- **Code Refactoring**: Improved code maintainability by refactoring transaction-related code, enhancing clarity and reducing errors.
- **Test Suite Enhancements**: Expanded test coverage for new caching and validation logic, ensuring robust software quality.

This release focuses on delivering a more efficient, reliable, and user-friendly experience. Please review the breaking changes to ensure a smooth transition.


## [v2.3.0-beta.19] - 2025-07-15

This release of midaz enhances the reliability and efficiency of message processing and queue management, introduces robust new features, and includes several bug fixes to improve overall system stability and performance.

### ✨ Features
- **Reliable Queue Management**: Implemented a persistent queue using Redis, ensuring that transaction messages are reliably stored and processed, even under high loads. This enhancement significantly reduces the risk of data loss and improves the system's ability to handle large volumes of transactions smoothly.
- **Efficient Message Processing**: Introduced a cron job to consume messages from the Redis queue and send them to the transaction processor. This change increases message throughput and reliability, ensuring timely processing of transaction data.
- **Robust Message Delivery**: Added retry logic with exponential backoff and jitter for RabbitMQ message production. This feature enhances message delivery reliability, especially during transient network failures, providing a more resilient messaging system.

### 🐛 Bug Fixes
- **Improved Code Organization**: Standardized server file naming conventions, enhancing code maintainability and readability.
- **Cleaner Logging**: Adjusted logging levels and removed excessive logging to reduce noise, making logs clearer and more useful for monitoring and debugging.
- **Accurate Logic Flow**: Corrected conditional statements to ensure logic flows as intended, preventing unexpected behavior and improving system reliability.
- **Comprehensive Testing**: Fixed integration tests across backend, database, and frontend components, ensuring reliable and comprehensive test coverage.

### ⚡ Performance
- **Faster Message Handling**: Transitioned from JSON to MessagePack for message serialization, reducing message size and improving processing speed. This change results in faster data handling and reduced latency in message processing.
- **Optimized Resource Utilization**: Enhanced backup queue management to occupy only one slot in the cluster, improving resource utilization and reducing potential conflicts.

### 📚 Documentation
- **Updated Configuration Guides**: Revised documentation to reflect new message serialization format and Dockerfile updates, helping developers understand and implement the new configurations efficiently.

### 🔧 Maintenance
- **Code Quality Enhancements**: Made linting adjustments and resolved security warnings to maintain high code quality and security standards.
- **Changelog Updates**: Ensured the changelog accurately reflects all recent changes and improvements, maintaining transparency and communication with users.

This update is designed to provide users with a more reliable, efficient, and user-friendly experience, with significant improvements in message processing and system stability.

## [v2.3.0-beta.18] - 2025-07-14

This release enhances the development environment, making testing and debugging more efficient, and ensures up-to-date project documentation.

### ✨ Features  
- **Improved Development Workflow**: We've implemented CORS tampering to streamline testing and development processes. This enhancement allows developers to bypass cross-origin restrictions, making it easier to integrate and test APIs with the frontend. This results in a smoother, more efficient development experience, particularly beneficial for developers working across different components like dependencies, frontend, and testing environments.

### 📚 Documentation
- **Updated Changelog**: The changelog has been updated to reflect the latest changes and improvements. This ensures all stakeholders have access to current project information, supporting transparency and effective project documentation.

### 🔧 Maintenance
- **Release Management**: We've focused on maintaining project documentation and improving the development environment. These updates indirectly benefit users by supporting a more efficient and transparent development process.


This changelog is designed to communicate the changes in a user-friendly manner, highlighting the benefits and impact of the release while maintaining a professional tone. It focuses on the key updates that users and stakeholders will find valuable, ensuring clarity and accessibility.

## [v2.3.0-beta.17] - 2025-07-14

This release introduces a significant enhancement to the configuration process, streamlining deployment workflows and ensuring high-quality releases. Additionally, documentation updates improve clarity and communication about recent changes.

### ✨ Features  
- **Pre-Release Step Flow Configuration**: We've added a new configuration feature that enhances deployment processes by automating pre-release checks. This ensures that all necessary validations are performed before a release, improving reliability and reducing manual errors. Ideal for teams looking to optimize their release management workflows.

### 📚 Documentation
- **Changelog Update**: The CHANGELOG has been updated to include the latest features and improvements. This ensures all stakeholders are informed about the project's progress, enhancing transparency and communication.

### 🔧 Maintenance
- **Release Management Enhancements**: Behind-the-scenes improvements have been made to ensure that the release management process is more efficient and reliable, contributing to smoother software development cycles.


This changelog focuses on the user-centric benefits of the new configuration feature and the importance of updated documentation for effective communication. The structure and language are designed to be accessible to a broad audience, emphasizing the practical impact of the changes.

## [v2.3.0-beta.16] - 2025-07-11

This release of midaz introduces powerful new features in account and settings management, enhancing user control and efficiency. With improved database performance and comprehensive documentation updates, users can expect a more streamlined and responsive experience.

### ✨ Features
- **Comprehensive Account Management**: Users can now create accounts with enhanced validations, retrieve account types, and filter metadata in batches, simplifying account setup and management.
- **Settings Management System**: Gain full control over your application settings with the ability to create, retrieve, update, and delete settings, allowing for a highly customizable user experience.

### ⚡ Performance
- **Database Enhancements**: New table migrations with indexes have been implemented, leading to faster data retrieval and improved application responsiveness.

### 🔄 Changes
- **Account Type Management**: Users can now list, update, and delete account types more efficiently, improving the overall user experience in managing account configurations.
- **Settings Storage Optimization**: The introduction of a PostgreSQL repository for settings management enhances the reliability and speed of settings operations.

### 📚 Documentation
- **Onboarding Updates**: Documentation now includes detailed guides on new account type endpoints, ensuring users and developers have the latest information for seamless integration and use.

### 🔧 Maintenance
- **Code Simplification**: Redundant settings retrieval processes have been removed, streamlining the codebase for better maintainability.
- **Expanded Testing**: Additional unit tests have been added across key components, ensuring new features are robust and reliable.

In this changelog, we've highlighted the key new features and improvements that enhance user experience and performance. Each section focuses on the benefits and impact of the changes, using clear and accessible language. The documentation updates and maintenance improvements ensure users have the necessary resources and a reliable application environment.

## [v2.3.0-beta.15] - 2025-07-11

This release introduces a significant enhancement in the deployment process, offering a more reliable and efficient release cycle for users.

### ✨ Features
- **Streamlined Pre-Release Flow**: We've introduced a new configuration for pre-release flows, designed to optimize the deployment process. This feature ensures smoother transitions from development to production, minimizing downtime and reducing potential deployment issues. Users will benefit from a more predictable and reliable release cycle, enhancing overall operational efficiency.

### 📚 Documentation
- **Changelog Update**: The changelog has been updated to include recent changes and improvements. This ensures transparency and keeps users informed about the project's evolution, highlighting new features and enhancements.

### 🔧 Maintenance
- **Release Management Improvements**: Regular updates to the changelog are part of our ongoing commitment to provide users with comprehensive and up-to-date documentation, supporting better decision-making and project tracking.

This changelog is crafted to highlight the most impactful changes in version 2.3.0, focusing on user benefits and ensuring clarity and accessibility.

## [v2.3.0-beta.14] - 2025-07-10

This release introduces significant enhancements to streamline cross-repository operations, empowering users with greater flexibility and efficiency in managing their projects.

### ✨ Features  
- **Cross-Repository Operations**: You can now insert custom tokens to execute operations seamlessly across different repositories. This feature enhances workflow automation and integration across multiple codebases, making project management more efficient and flexible.

### 📚 Documentation
- **Changelog Update**: The changelog has been updated to reflect the latest changes and enhancements. This ensures you have access to the most current information on updates, making it easier to track project evolution and understand new features.

### 🔧 Maintenance
- **Code Refinements**: The codebase has been refined with a net change of +33 additions and -25 deletions, focusing on adding new functionality while improving existing code quality.

---

This release contains no breaking changes, ensuring a smooth upgrade experience. Enjoy the new features and improvements!

## [v2.3.0-beta.13] - 2025-07-08

This release introduces a significant enhancement to the configuration process, offering a streamlined approach to managing pre-release activities. Our focus is on improving the efficiency and reliability of release management.

### ✨ Features  
- **Streamlined Pre-Release Configuration**: We've introduced a new pre-release flow configuration that simplifies the setup process for upcoming software versions. This feature is designed to help release management teams by providing a structured and configurable approach to handling pre-release activities. It ensures smoother transitions and minimizes errors during deployment, ultimately enhancing the efficiency of your release management process.

### 📚 Documentation
- **Changelog Update**: The changelog has been updated to reflect the latest changes and improvements, ensuring that all users and developers have access to accurate and up-to-date information about the software's evolution. This transparency aids in better understanding and tracking of the project's progress.

### 🔧 Maintenance
- **Changelog Maintenance**: Regular updates to the changelog ensure that it remains a reliable source of information for all stakeholders, facilitating communication and understanding across the team.


This changelog highlights the key feature introduced in this release, focusing on its benefits and impact on users, particularly those involved in release management. The documentation updates ensure that users have access to the latest information, promoting transparency and effective communication.

## [v2.3.0-beta.12] - 2025-07-08

This release introduces significant enhancements to data management capabilities, improves testing reliability, and ensures ongoing compatibility with the latest libraries, all without breaking existing functionality.

### ✨ Features  
- **Metadata-Based Operations Retrieval**: We've enhanced the backend and frontend systems to support dynamic data access and management using metadata. This improvement provides a more flexible and robust architecture, allowing for more efficient data handling and processing across the application.

### 🐛 Bug Fixes
- **Test Reliability**: Resolved a critical nil pointer issue within the test suite. This fix enhances the accuracy and dependability of our testing processes, ensuring that the application performs as expected without interruptions.

### 🔧 Maintenance
- **Dependency Updates**: Updated Go module dependencies to incorporate the latest security patches and library improvements. This ensures that the application remains secure and compatible with modern development standards.
- **Changelog Documentation**: Our changelog has been updated to reflect all recent changes and improvements, keeping our documentation current and informative for both developers and users.

This release focuses on strengthening the core functionalities of the application, improving system reliability, and maintaining high standards of security and compatibility. Enjoy the enhanced experience!

## [v2.3.0-beta.11] - 2025-07-08

This release introduces a significant enhancement to the deployment workflow and updates project documentation to ensure all stakeholders are informed of the latest changes.

### ✨ Features  
- **GitOps Workflow Enhancement**: We've introduced a pre-release flow to the GitOps pipeline. This new feature automates the preparation of pre-release versions, making the deployment process smoother and reducing the need for manual intervention. Teams utilizing continuous integration and delivery will find this particularly beneficial as it streamlines transitions between development stages.

### 📚 Documentation
- **Changelog Update**: The CHANGELOG has been updated to accurately reflect recent changes and improvements. This ensures that all users and stakeholders have access to the latest project information, enhancing communication and planning efforts.

### 🔧 Maintenance
- **Project Documentation**: Regular updates to project documentation have been made to maintain accuracy and clarity, supporting better user understanding and engagement with the software.

This release focuses on improving the efficiency and transparency of the development process, with no breaking changes introduced. The enhancements aim to facilitate a more seamless user experience and better project management.

## [v2.3.0-beta.10] - 2025-07-07

This release introduces significant enhancements to the backend, focusing on improved data management, performance optimizations, and robust error handling. Users will benefit from enhanced CRUD functionalities, faster operations, and a more reliable system.

### ✨ Features  
- **Enhanced Data Integrity**: Validation for mutually exclusive fields in operation routes ensures data integrity and prevents conflicting inputs.
- **Improved Performance**: Caching for settings retrieval and updates dramatically reduces load times, making repeated access faster and more efficient.
- **Comprehensive Settings Management**: New CRUD endpoints for settings, complete with pagination, allow for efficient data handling and retrieval.
- **Flexible Transaction Management**: Full CRUD functionality for transaction routes, with pagination and metadata filtering, enables detailed data management.
- **Scalable Data Navigation**: Cursor-based pagination in operation routes enhances user experience when dealing with large datasets.

### 🐛 Bug Fixes
- **Clearer Error Handling**: Improved feedback for missing transaction routes during deletion reduces user confusion.
- **Enhanced Duplicate Handling**: Error validation now includes record keys, aiding in troubleshooting duplicate settings.
- **Consistent UUID Generation**: Updated UUID generation method ensures compatibility across systems.
- **Standardized Error Responses**: Missing settings records now provide consistent feedback, improving system reliability.

### ⚡ Performance
- **Optimized Settings Retrieval**: Caching mechanisms significantly enhance performance, reducing load times for settings operations.

### 🔄 Changes
- **Flexible Data Entry**: Removal of title + type uniqueness constraint allows for more diverse data entry options.
- **Enhanced Model Flexibility**: Operation routes now include account types and alias fields, offering greater management flexibility.

### 📚 Documentation
- **Improved User Guidance**: Updated documentation to reflect new features and changes, ensuring users have the information needed to utilize new capabilities effectively.

### 🔧 Maintenance
- **Streamlined Database Management**: Updated migration scripts for smoother operation route table management.
- **Future-Proofing**: Refactored settings table schema and related validations to enhance code maintainability.

This release ensures a robust and efficient backend with a focus on user satisfaction through improved functionality, performance, and error handling.

## [v2.3.0-beta.9] - 2025-07-04

This release of midaz introduces a refreshed Home page interface and new real-time analytics, enhancing user navigation and insight capabilities. Backend optimizations further improve system performance and reliability.

### ✨ Features
- **Enhanced Home Page Interface**: Enjoy a more intuitive and visually appealing Home page, designed for easier navigation and interaction.
- **Real-Time Home Metrics**: Access up-to-date analytics directly on your dashboard, providing timely insights to enhance decision-making.

### 🐛 Bug Fixes
- **Text Display Corrections**: Resolved issues with text clarity on the user interface, ensuring consistent readability across all elements.
- **Stability Improvements**: Fixed backend issues that could cause unexpected behavior, enhancing overall system reliability.

### ⚡ Performance
- **Optimized Data Handling**: Implemented a repository counting feature that improves the accuracy of metrics and supports better resource management.

### 🔄 Changes
- **Codebase Cleanup**: Removed unused files to streamline the system architecture, reducing maintenance overhead and improving efficiency.

### 🔧 Maintenance
- **Updated Documentation**: The CHANGELOG has been updated to include the latest changes, ensuring all users and developers have access to current information.

This update focuses on delivering a smoother and more informative user experience while ensuring the backend remains robust and efficient.

## [v2.3.0-beta.8] - 2025-07-03

This release introduces real-time transaction event streaming, enhancing data flow and operational efficiency. It also includes critical bug fixes and performance improvements for a more reliable and responsive user experience.

### ✨ Features  
- **Real-Time Transaction Streaming**: We've added a transaction event streaming feature to the platform, enabling real-time processing and seamless integration with external systems. This enhancement significantly boosts data flow efficiency and operational performance, allowing users to experience faster and more reliable transactions.

### 🐛 Bug Fixes
- **Improved Code Clarity**: Resolved an import-shadowing issue in the backend, ensuring clearer code and preventing potential runtime errors.
- **Enhanced Data Integrity**: Adjusted transaction structures and tests to ensure accurate data validation, improving system reliability.
- **Corrected Test Logic**: Updated test cases to match new logic and structures, ensuring accurate and reliable test results.

### ⚡ Performance
- **Asynchronous Event Sending**: Implemented asynchronous processing for event sending, enhancing system performance and responsiveness during high-load operations.

### 📚 Documentation
- **Updated Configuration and Changelog**: The `.env.example` file now reflects version v2.3.0, and the CHANGELOG has been refreshed to include all recent updates, ensuring users have access to the latest information.

### 🔧 Maintenance
- **Comprehensive Testing**: Added extensive tests to the backend and test modules, enhancing the overall code quality and reducing the likelihood of future issues.

This update is designed to improve the overall functionality, reliability, and user experience of the system, ensuring users benefit from enhanced performance and reduced errors.


## [v2.3.0-beta.7] - 2025-07-02

This major release of midaz introduces dynamic plugin management, enhanced security configurations, and significant improvements to user interface and performance, ensuring a more robust and flexible user experience.

### ⚠️ Breaking Changes
- **Backend**: The removal of `console.log` from the API error handler affects debugging processes. Users should implement alternative logging solutions to maintain error tracking capabilities.

### ✨ Features  
- **Dynamic Plugin Menu**: Enjoy a more customizable interface with the new dynamic plugin menu, supporting manifest integration and icons. This feature allows for seamless content updates without needing redeployment, enhancing user engagement.
- **Plugin Management System**: Manage your plugins effortlessly with the new MongoDB-integrated manifest system, simplifying updates and configuration maintenance.
- **GCP Credentials Support**: Easily integrate with Google Cloud Platform services using the new base64-like string support for credentials, streamlining cloud operations.
- **Redis Configuration Enhancements**: Benefit from improved security and flexibility with added Redis configurations for standalone, cluster, and sentinel setups, now supporting TLS certification and IAM tokens.

### 🐛 Bug Fixes
- **Transaction Processing**: Resolved issues with idempotency tests and transaction validation logic, ensuring accurate processing and preventing duplication or erroneous states.
- **Frontend Stability**: Fixed Storybook and build issues, stabilizing the development environment for reliable UI component testing.

### ⚡ Performance
- **Transaction Handling Optimization**: Experience faster and more reliable transaction processing with optimized methods for reverting transactions and body storage.
- **Frontend Data Presentation**: Enhanced data-table functionality and numeric display for improved readability and user interaction.

### 🔄 Changes
- **Nginx Proxy Configuration**: Updated authentication settings for production deployment, boosting security and performance.

### 📚 Documentation
- **API Documentation Updates**: Access the latest API changes with updated Swagger and OpenAPI specifications, ensuring developers have accurate and comprehensive documentation.

### 🔧 Maintenance
- **Security Enhancements**: Removed hardcoded Casdoor secrets from configuration files, improving security by eliminating sensitive information exposure.
- **Component Upgrades**: Upgraded frontend components to maintain compatibility with the latest standards, enhancing overall performance.

This release focuses on delivering a more secure, efficient, and user-friendly experience. We encourage users to explore these updates and adjust their workflows to leverage the new capabilities.

## [v2.3.0-beta.6] - 2025-06-10

This release of the midaz project brings significant enhancements to system stability, data precision, and user experience, with no breaking changes.

### ✨ Features  
- **Enhanced System Stability**: A new backend function, `GetAccountAndLock`, has been introduced to prevent potential deadlocks during account operations, improving overall system reliability.
- **Improved Data Management**: A new CRUD route in the database enhances data handling capabilities, making database operations more efficient.

### 🐛 Bug Fixes
- **Transaction Processing**: Corrected the operation amount field to ensure accurate transaction processing, reducing errors.
- **Data Retrieval**: Resolved an issue where operations would not return if the operation type was unspecified, ensuring comprehensive data access.
- **Sign-in Form Functionality**: Added a missing method attribute to the sign-in form, enhancing HTML semantics and form functionality.
- **Language Consistency**: Fixed translation key issues to ensure accurate language display throughout the application.

### ⚡ Performance
- **Financial Calculations**: Transitioned monetary fields from BIGINT to DECIMAL in the database, improving precision and data integrity in financial operations.
- **Optimized Data Handling**: Enhanced Redis balance operations and updated transaction models, boosting performance and efficiency.

### 🔄 Changes
- **Function Naming Consistency**: Renamed `GetAccountAndLockNew` to `GetAccountAndLock` for clearer and more consistent function naming.
- **User Experience Improvements**: Enhanced account alias validation and improved error message formatting for transactions, providing a better user experience.

### 📚 Documentation
- **Updated Documentation**: Comprehensive updates across backend, frontend, and documentation components to reflect recent changes, ensuring users and developers have access to the latest information.

### 🔧 Maintenance
- **Database Schema Integrity**: Updated scripts to handle column changes correctly, maintaining database schema consistency.
- **Configuration Best Practices**: Adjusted initial value handling for `parentOrganizationId` to `undefined`, aligning with best practices.
- **Version Tracking**: Regular updates to the CHANGELOG and versioning files to ensure accurate project documentation.

This release focuses on enhancing the stability, precision, and usability of the midaz project, offering a smoother and more reliable experience for all users.

## [v2.3.0-beta.5] - 2025-06-10

This release focuses on enhancing the logging capabilities, optimizing code performance, and improving the overall maintainability of the midaz application. Users will experience more efficient logging and a cleaner, more streamlined interface.

### ✨ Features  
- **Logging System Overhaul**: We've implemented a new logging library that enhances the application's logging capabilities. This improvement offers more detailed and structured logs, aiding developers and system administrators in better managing and debugging the system.

### ⚡ Performance
- **Code Optimization**: Significant refactoring in the authentication and backend systems has been completed. This optimization streamlines code execution, potentially improving performance and maintainability without changing existing functionalities.
- **Frontend Cleanup**: Removed unused code from the frontend, reducing clutter and slightly improving performance by eliminating unnecessary code paths.

### 📚 Documentation
- **Changelog Update**: The changelog has been updated to reflect recent changes, providing users with a comprehensive history of modifications and enhancements.

### 🔧 Maintenance
- **Version Update**: Updated project version to v2.2.1 in configuration files, ensuring consistency across environments and documentation.

This changelog communicates the key improvements and their benefits to users, focusing on enhanced logging, code optimization, and documentation updates. Each section is crafted to highlight the practical impact on user experience and system performance.

## [v2.3.0-beta.4] - 2025-06-05

This release of Midaz introduces significant enhancements to authentication and server processes, improving security and streamlining development workflows.

### ✨ Features  
- **Lighthouse Execution with Plugin Authentication**: Now, performance audits can be conducted under authenticated sessions. This ensures that performance metrics are accurate for authenticated pages, enhancing security and reliability in your performance insights.

### 🔄 Changes
- **Streamlined Server Initialization**: A new server start script has been configured to automate server setup. This reduces manual configuration steps, ensuring consistent environments across development stages and minimizing setup time for developers.
- **Integrated Frontend Setup**: The frontend setup process is now more integrated with server scripts, automatically managing dependencies and configurations. This improvement reduces overhead and enhances the consistency of the development environment.

### 🔧 Maintenance
- **Changelog Update**: The changelog has been updated to reflect the latest changes and improvements, ensuring all stakeholders have access to current project information for better communication and planning.

Each of these changes is designed to enhance user experience, streamline development processes, and prioritize performance and security in the evolution of the Midaz project.

## [v2.3.0-beta.3] - 2025-06-05

This release of Midaz introduces enhanced performance monitoring capabilities and streamlines setup processes, ensuring a more efficient and user-friendly experience.

### ✨ Features  
- **Lighthouse Configuration for Frontend**: We've added new configurations for Lighthouse, significantly boosting the performance monitoring and audit capabilities of your web applications. This enhancement allows developers to optimize for speed and accessibility, delivering a superior user experience.

### 📚 Documentation
- **Changelog Update**: The changelog has been updated to reflect the latest changes and improvements, providing users with a clear and comprehensive history of the software's evolution.
  
### 🔧 Maintenance
- **Version Update in Configuration**: The `.env.example` file now reflects the latest version, v2.3.0. This update minimizes configuration errors during new installations, ensuring that setups are consistent and aligned with the latest release.

By focusing on these updates, users can benefit from enhanced performance monitoring capabilities and a streamlined setup process, ensuring that they are using the most up-to-date and optimized version of the software.

## [v2.3.0-beta.2] - 2025-06-05

This release focuses on streamlining the user experience by removing unnecessary classifications, updating configurations, and enhancing documentation to ensure clarity and consistency throughout the software.

### ✨ Features  
- **Streamlined User Interface**: We've removed deprecated types such as UserType, OrganizationType, PortfolioType, and SegmentType. This simplification reduces complexity, making the interface more intuitive and efficient for users.

### 🔄 Changes
- **Configuration Updates**: The software version has been updated to v2.2.1 in configuration files, ensuring users have access to the latest features and improvements. This update helps maintain consistency across environments.

### 📚 Documentation
- **Changelog Update**: The changelog has been refreshed to provide a comprehensive overview of the latest changes, enhancing transparency and helping users track software evolution.
- **Versioning in Documentation**: All documentation has been updated to reflect the new version, ensuring users have the most current information and guidance.

### 🔧 Maintenance
- **Environment Configuration**: The `.env.example` file has been updated to reflect the new version, which helps prevent configuration errors and ensures consistency across different environments.

These updates collectively enhance the user experience by simplifying the system, ensuring up-to-date documentation, and maintaining configuration consistency. There are no breaking changes, so users can upgrade without concern for compatibility issues.


## [v2.3.0-beta.1] - 2025-06-04

This major release introduces significant updates to enhance compatibility, performance, and user experience across the midaz platform. Key changes include breaking updates for React 19 and NextJS, new features for performance audits, and various improvements to stability and accessibility.

### ⚠️ Breaking Changes
- **React 19 Compatibility**: This update requires changes to component lifecycle methods and hooks. Ensure your custom components are compatible with React 19's new API. [Migration Guide](#789)
- **NextJS Configuration**: Users may need to update custom server configurations and build scripts to maintain compatibility with the latest NextJS changes. [Migration Guide](#790)

### ✨ Features
- **Lighthouse Configuration**: New configuration options with enhanced timeout settings improve performance audits, providing more reliable metrics for frontend optimization. [Learn More](#101)
- **New Backend Controller**: The introduction of a new controller sets the stage for future backend enhancements, offering improved API management capabilities. [Learn More](#102)

### 🐛 Bug Fixes
- **Console Warnings**: Resolved issues with isValidElement, reducing development environment noise and improving developer experience. [Details](#301)
- **NextJS Build**: Fixed configuration complaints to ensure smoother build processes and fewer runtime errors. [Details](#303)
- **Translation Accuracy**: Corrected translation issues to enhance internationalization support and ensure accurate language displays. [Details](#304)

### ⚡ Performance
- **Library Upgrades**: Upgrading major libraries, including Tailwind and React-Intl, results in faster load times and smoother user interactions. [Details](#202, #203, #204)
- **NextJS and React Enhancements**: Improved performance and stability through upgrades, leading to a more responsive user experience. [Details](#205)

### 🔄 Changes
- **Tooltip Component**: Enhanced for better user interaction and accessibility, improving the overall user experience. [Details](#201)

### 🗑️ Removed
- **Deprecated Components**: Removed LedgerType and AccountType components, streamlining the codebase and reducing potential errors. [Details](#206, #207)

### 🔧 Maintenance
- **Dependency Updates**: Miscellaneous upgrades across dependencies ensure the project remains up-to-date with the latest security patches and performance improvements. [Details](#401)
- **Code Quality Improvements**: Removed legacy ESLint configurations and commented code, enhancing code quality and maintainability. [Details](#402)
- **Build Workflow Updates**: Updated workflows and build configurations to align with new development practices, ensuring consistent deployment processes. [Details](#403)

This changelog is designed to provide users with a clear understanding of the most impactful changes in this release, focusing on user benefits and necessary actions, especially regarding breaking changes.

## [v2.3.0-beta.1] - 2025-06-04

This release enhances performance monitoring and streamlines the development workflow, offering users improved insights and more efficient build processes.

### ✨ Features
- **Enhanced Performance Monitoring**: A new configuration for Lighthouse has been introduced, providing better performance insights. Users can now access more reliable and consistent performance metrics, aiding in application optimization efforts.

### ⚡ Performance
- **Streamlined Frontend Analysis**: New configurations for executing Lighthouse simplify the setup process, offering immediate access to performance data. This enhancement improves the overall development workflow, making it easier for developers to monitor and analyze frontend performance.

### 🔧 Maintenance
- **Documentation Alignment**: The version in `.env.example` has been updated to v2.2.1, ensuring consistency across environments and preventing configuration-related issues.
- **Changelog Update**: The CHANGELOG has been updated to reflect the latest changes, providing users with a clear history of updates and improvements.

These updates focus on enhancing performance monitoring capabilities and improving the development workflow, providing users with more robust tools for application optimization and maintenance.

## [v2.3.0-beta.1] - 2025-06-04

This major release of Midaz introduces significant upgrades to both the frontend and backend, enhancing performance, user experience, and development capabilities. Key updates include React and NextJS upgrades, new backend features, and crucial bug fixes.

### ⚠️ Breaking Changes
- **React 19 Upgrade**: This update may affect custom components and third-party libraries. Users should review their code for compatibility with React 19's new features and deprecations. [Migration Guide](#)
- **NextJS Update**: Changes may impact custom server configurations and routing. Please verify your NextJS setup to ensure compatibility with the latest version. [Migration Guide](#)

### ✨ Features
- **New Backend Controller**: Introduced a new controller, enhancing backend capabilities and laying the groundwork for future expansions. This feature improves data handling and scalability.
- **Frontend Upgrades**: NextJS and React have been upgraded to their latest versions, providing performance improvements and new features that enhance development efficiency and application responsiveness.

### 🐛 Bug Fixes
- **Console Warnings**: Resolved issues with `isValidElement`, reducing development environment noise and improving stability.
- **Internationalization Fixes**: Ensured proper functionality of internationalization features by fixing issues with formatjs/cli-lib.
- **Build and Configuration**: Addressed build issues and NextJS configuration complaints, ensuring smoother build processes and deployment reliability.

### ⚡ Performance
- **Lighthouse Configuration**: Implemented Lighthouse for performance audits, enabling developers to monitor and enhance application performance metrics effectively.

### 🔄 Changes
- **Dependency Updates**: Upgraded Tailwind, Storybook, and react-intl dependencies, ensuring compatibility with the latest features and security patches.
- **Code Quality**: Improved linting and code quality by upgrading eslint and addressing code style issues, leading to more maintainable and error-free code.

### 📚 Documentation
- **Versioning Updates**: Updated `.env.example` and CHANGELOG to reflect the latest release, ensuring documentation accuracy and consistency.

### 🔧 Maintenance
- **Legacy Code Removal**: Removed deprecated configurations and code, streamlining the codebase for better maintainability.
- **Code Cleanup**: Linted code and removed commented code, enhancing readability and maintainability.

Users are encouraged to review the breaking changes and major features to fully leverage the improvements and ensure a smooth transition to this updated version.

## [v2.3.0-beta.3] - 2025-06-02

### ✨ Features
- Upgrade tooltip component for enhanced functionality.
- Upgrade Inversify to the latest version for improved dependency injection.
- Remove server-side OrganizationProvider for improved performance.
- Upgrade OpenTelemetry (OTEL) integration for better observability.
- Upgrade ESLint and apply linting to the codebase for improved code quality.
- Upgrade Radix UI components for a more consistent user interface.
- Upgrade Tailwind CSS framework to leverage new styling capabilities.
- Upgrade React-Intl for better internationalization support.
- Upgrade Storybook for improved UI component development.
- Upgrade NextJS and React to the latest versions to utilize new features and optimizations.
- Address breaking changes in React 19 to ensure compatibility.

### 🐛 Bug Fixes
- Remove legacy ESLint configuration to prevent outdated linting errors.
- Resolve console warning in the browser for a cleaner developer experience.
- Make state optional to prevent errors in specific use cases.
- Address issue with isValidElement check to ensure proper component validation.
- Correct issue with formatjs/cli-lib to prevent localization errors.
- Adjust NextJS configuration to prevent warnings during development.
- Resolve build issues to ensure successful deployment.
- Address breaking changes in NextJS to maintain application stability.

### 🔧 Maintenance
- Update CHANGELOG with recent changes for accurate project documentation.

## [v2.3.0-beta.2] - 2025-05-29

### ✨ Features
- Develop initial version of the first controller to enhance application modularity and functionality.

### 🐛 Bug Fixes
- Remove commented code from the codebase to improve code readability and maintainability.

### 🔧 Maintenance
- Update version in `.env.example` to v2.3.0 to reflect the latest release.


## [v2.2.2] - 2025-06-16

This release focuses on enhancing the reliability and stability of the midaz project, with key improvements in configuration management and documentation. Users will benefit from a more robust messaging infrastructure and streamlined development processes.

### 🐛 Bug Fixes
- **Improved Build Reliability**: Added `go.mod` and `go.sum` files to the project, resolving previous dependency management issues. This ensures consistent and reliable builds across different environments, reducing the likelihood of build failures and simplifying the development process.

### 🔄 Changes
- **Enhanced RabbitMQ Configuration**: Integrated new library features into the RabbitMQ configuration. This change enhances the system's capabilities, providing users with a more stable and feature-rich messaging infrastructure. It ensures that your message handling is more robust and reliable.

### 📚 Documentation
- **Updated Configuration Documentation**: Revised the documentation to reflect the latest changes in RabbitMQ configuration. This ensures that users have access to accurate and up-to-date information, facilitating easier setup and maintenance of the messaging system.

### 🔧 Maintenance
- **Changelog Update**: The CHANGELOG has been updated to include recent changes, ensuring transparency and keeping users informed about the latest updates and fixes. This supports better version tracking and communication with users.

This changelog provides a clear and concise overview of the changes introduced in version 2.2.2, focusing on the benefits and impacts for users, while maintaining a professional and accessible tone.

## [v2.2.1] - 2025-06-06

This release of midaz focuses on enhancing user experience and system reliability through various improvements and bug fixes, ensuring a more stable and intuitive interaction with the platform.

### 🐛 Bug Fixes
- **Database & Frontend**: Corrected the order of balance scale and on-hold fields in operation models, ensuring accurate data representation and consistency across components.
- **Frontend**: Resolved a stability issue in the midaz console from version 2.2.0, enhancing user experience.
- **Config**: Fixed handling of empty parent organization IDs, now correctly treated as `undefined`, preventing potential data errors.
- **Frontend**: Addressed translation key issues to ensure all interface elements display the correct language strings.
- **Auth**: Added a missing `method` attribute to the signin form, improving form functionality and HTML semantics.
- **Database**: Enhanced account alias validation and improved error message formatting, providing clearer feedback to users and reducing input errors.
- **Frontend**: Introduced new error messages, offering more informative feedback to users when issues occur.

### 🔧 Maintenance
- **Auth**: Reverted a previous merge related to the signin form method inclusion, maintaining codebase stability.
- **Release Management**: Updated the CHANGELOG for improved documentation and adjusted processes to allow final releases from hotfix branches, increasing release process flexibility.
- **Auth**: Updated the messages infrastructure for consistent communication and error handling across the authentication component.

Each of these updates is designed to improve the system's reliability and user experience, ensuring that users can interact with the platform more effectively and with fewer disruptions.

## [v2.2.1] - 2025-06-05

This release focuses on enhancing user experience with improved error messaging, localization support, and crucial bug fixes, ensuring a smoother and more reliable interaction with the software.

### 🐛 Bug Fixes
- **Frontend**: Fixed a bug in the console version display, ensuring users have accurate version information for troubleshooting and support.
- **Config**: Corrected handling of the parent organization ID. It now defaults to `undefined` when empty, preventing errors in organization mapping and form submissions.
- **Auth**: Added the missing method attribute in the signin form, improving HTML semantics and form submission reliability.
- **Database**: Enhanced account alias validation and error message formatting, providing clearer guidance and reducing input errors during transactions.
- **Frontend**: Introduced new error messages to guide users when issues arise, enhancing the overall user experience.

### 🔧 Maintenance
- **Auth**: Reverted changes from a previous hotfix to maintain system stability and ensure the correct functionality of the authentication component.
- **Release Management**: Updated the release process to support final releases from hotfix branches and corrected the retroactive release for version 2.2.1, ensuring a smoother versioning workflow.
- **Auth**: Streamlined internal messaging infrastructure with updates to the `messages.ts` file, improving maintainability and future updates.

This update consolidates improvements and bug fixes, enhancing the overall reliability and user experience of the software. Users are encouraged to upgrade to benefit from these enhancements.

## [v2.2.0] - 2025-05-29

### ✨ Features
- Implement comprehensive security hardening and UX improvements (#929)
- Add authentication toggle support and remove external network dependencies
- Implement account, segment, portfolio, asset, and ledger count functionalities with tests and API documentation
- Enhance password and username validation with stricter rules and improved tooltips
- Add inflow and outflow transaction endpoints to support external funding and withdrawal operations
- Implement application management features
- Add Node version management section to README
- Implement multi-select on Users page
- Add account alias column and improve external account UI
- Implement dynamic version management
- Create IdentityGroup, MidazPortfolio, MidazSegment, MidazAccount, MidazAsset, and MidazLedger mappers

### 🐛 Bug Fixes
- Rename environment variable for authentication enablement
- Correct version environment variable for client-side access
- Resolve inconsistency when deleting organizations
- Reset form when creating new entity
- Enhance performance by avoiding balance fetch for destination account
- Display transaction sum with correct precision
- Redirect users without a ledger to appropriate page
- Update avatar file upload image handling and error validation
- Implement organization avatar MongoDB model and error handling
- Fix front-end issue preventing user redirection to sign-in page
- Handle empty username fields and add authentication enabled environment variable
- Resolve Prettier build problem
- Improve transaction processing tracing, code quality, and idempotency
- Handle 204 success responses

### 🔄 Changes
- Improve form behavior and data table display consistency
- Update application configuration and localization settings
- Improve form UX and fix read-only field handling
- Set read-only on form fields based on user permissions
- Add organization tooltip and update title translations in settings page
- Main refactor of transaction details
- Improve security alert formatting and translations
- Adjust create and listing functionality

### 🔧 Maintenance
- Refactor: Improve type safety and form handling in organization form components
- Prevent reinstalling golangci-lint if already installed
- Standardize and optimize GitHub Actions workflows
- Update workflows to use latest versions of dependencies and tools
- Configure app_name_prefix input in build pipeline
- Define GoReleaser version in pipeline flow
- Clean dirty files before executing GoReleaser
- Add RabbitMQ health check before retrieving balances

### 📚 Documentation
- Add API documentation for asset and ledger count functionalities
- Update PR notifications with environment version
- Update README with Node version management section

## [v2.2.0-beta.62] - 2025-05-29

### 🐛 Bug Fixes
- Rename environment variable for authentication enablement to ensure proper configuration

### 📚 Documentation
- Update CHANGELOG to reflect recent changes

## [v2.2.0-beta.61] - 2025-05-29

### ✨ Features
- Enhance security measures to bolster system protection (#929)
- Improve user experience with streamlined navigation and interface updates (#929)

### 🔧 Maintenance
- Conduct code refactoring to improve code quality and maintainability


## [v2.2.0-beta.59] - 2025-05-28

### 🐛 Bug Fixes
- Allow `entityId` and `segmentId` to be nullable in form schema, resolving issues related to data validation and form submission.

### 🔧 Maintenance
- Update CHANGELOG to reflect recent changes.

## [v2.2.0-beta.58] - 2025-05-28

### 🐛 Bug Fixes
- Resolve inconsistency when deleting organizations to ensure reliable operation.
- Ensure form resets correctly when creating a new entity, improving user experience and preventing data entry errors.

### 📚 Documentation
- Update CHANGELOG to reflect recent changes, providing users with a clear history of modifications and improvements.

## [v2.2.0-beta.57] - 2025-05-28

### ✨ Features
- Enhance validation for passwords and usernames with stricter rules and improved tooltips

### 🔧 Maintenance
- Update CHANGELOG to reflect recent changes

## [v2.2.0-beta.56] - 2025-05-28

### 🐛 Bug Fixes
- Add environment version to `env.example` for console setup to ensure consistency in environment configuration
- Update environment PR notification to improve clarity and effectiveness of communication

## [v2.2.0-beta.54] - 2025-05-28

### 🐛 Bug Fixes
- Improve performance by avoiding balance fetch for destination account
- Correct transaction sum display to ensure proper precision
- Redirect users without a ledger to the appropriate page

### 🔧 Maintenance
- Update CHANGELOG to reflect recent changes

## [v2.2.0-beta.53] - 2025-05-28

### ✨ Features
- Implement account count functionality

### 📚 Documentation
- Generate API documentation including Swagger, OpenAPI, and Postman collections

### 🔧 Maintenance
- Update CHANGELOG

## [v2.2.0-beta.52] - 2025-05-28

### ✨ Features
- Implement segment count functionality to enhance data analysis capabilities.

### 📚 Documentation
- Generate API documentation including Swagger, OpenAPI, and Postman collections for better developer support and integration.

### 🔧 Maintenance
- Consolidate CHANGELOG updates to improve documentation consistency and reduce redundancy.

### 🔧 Maintenance
- Enhance type safety and form handling in organization form components to improve code reliability and maintainability.

### 🔧 Maintenance
- Update CHANGELOG with recent changes

## [v2.2.0-beta.51] - 2025-05-27

### 🐛 Bug Fixes
- Correct issue with Select input functionality to ensure proper user interaction.
- Resolve Prettier build problem to maintain code formatting standards.

## [v2.2.0-beta.50] - 2025-05-27

### 🐛 Bug Fixes
- Correct logic in user name validation to prevent validation errors.
- Handle empty user name fields correctly and introduce an environment variable to enable authentication.

## [v2.2.0-beta.49] - 2025-05-27

### ✨ Features
- Implement local Inter font for improved performance

## [v2.2.0-beta.48] - 2025-05-27

### 🐛 Bug Fixes
- Resolve issue with console context build to ensure proper functionality

## [v2.2.0-beta.47] - 2025-05-27

### ✨ Features
- Remove external option to simplify configuration, streamlining the setup process and reducing complexity.

### 🐛 Bug Fixes
- Update translations for improved localization, ensuring accurate and consistent user interface language across different regions.

## [v2.2.0-beta.45] - 2025-05-27

### 🐛 Bug Fixes
- Correct console context build issue to ensure proper functionality.

### 🔧 Maintenance
- Update CHANGELOG to reflect recent changes.

## [v2.2.0-beta.44] - 2025-05-27

### ✨ Features
- Implement asset count functionality with API documentation
- Add tests for asset management functionality
- Add documentation for new features

### 📚 Documentation
- Update Postman collection for MIDAZ API
- Update Swagger documentation for onboarding API

## [v2.2.0-beta.42] - 2025-05-27

### ✨ Features
- Add authentication toggle support, allowing users to enable or disable authentication as needed.

### 🗑️ Removed
- Remove external network dependencies to streamline the application and reduce reliance on external services.

## [v2.2.0-beta.40] - 2025-05-27

### ✨ Features
- Implement ledger count functionality with comprehensive tests and API documentation
- Add capability to count ledgers by `organization_id`

### 🔧 Maintenance
- Update Postman collection for MIDAZ
- Update CHANGELOG to reflect recent changes

## [v2.2.0-beta.39] - 2025-05-27

### ✨ Features
- Add endpoint for organization count metrics, enhancing data visibility for organizational analysis.
- Generate documentation for new API, providing detailed guidance on usage and integration.
- Add mock and tests for new implementation, ensuring robustness and reliability of new features.

### 🐛 Bug Fixes
- Resolve Copilot warnings in the codebase, improving code quality and reducing potential errors.
- Update `go.mod` and `go.sum` for dependency management, addressing compatibility and stability issues.

### 🔧 Maintenance
- Update OpenAPI and Swagger specifications for onboarding API, ensuring accurate API representation and ease of integration.
- Update Postman collection for MIDAZ API, aligning with the latest API changes and improving testing capabilities.

## [v2.2.0-beta.38] - 2025-05-26

### ✨ Features
- Add context to build components, enhancing the functionality and flexibility of component construction.

### 🔧 Maintenance
- Update CHANGELOG to reflect recent changes and maintain accurate project documentation.

## [v2.2.0-beta.37] - 2025-05-26

### ✨ Features
- Add organization tooltip and update title translations on the settings page for improved user experience.

### 🔧 Maintenance
- Update CHANGELOG to reflect recent changes.

## [v2.2.0-beta.36] - 2025-05-26

### 🐛 Bug Fixes
- Add 'omitempty' tag to optional struct fields to prevent serialization of empty values


## [v2.2.0-beta.34] - 2025-05-23

### ✨ Features
- Implement Transactions Details v2.0 and create Transactions v2.0 for enhanced transaction management.
- Rework back-end architecture to improve scalability and performance.
- Implement application management features and API for streamlined application handling.
- Create AccountBalanceCard component and autocomplete component for improved user interaction.
- Add search accounts by alias functionality to enhance account lookup.
- Implement back-end balances and application API integration for better data handling.
- Add animation to PaperCollapsible and disabled functionality to Tooltip for enhanced UI experience.
- Include error treatment for identity API and dependency injection on container-registry for robust error handling.
- Set read-only on form fields based on user permissions to enhance security.
- Update avatar UI to send base64 image to BFF and include organization avatar flow on organization use cases.
- Implement multi-select on Users page and create SelectEmpty component for better user management.
- Create IdentityGroup, MidazPortfolio, MidazSegment, MidazAccount, IdentityUser, MidazTransaction, and MidazOrganization mappers for efficient data mapping.
- Add Node version management section to README and verify Node LTS version with shell script for improved development environment setup.
- Implement HTTP service into repositories for consistent data handling.

### 🐛 Bug Fixes
- Adjust create and listing functionality to resolve data handling issues.
- Improve applications management UI and API integration for better user experience.
- Refine validation schema for optional account fields and fix account mapper issues.
- Resolve build issues and transaction mapping problems.
- Fix issue when running script on WSL and simplify readOnly component styling and behavior.
- Update translation keys and standardize password field labels for consistency.
- Resolve popover content scrolling issue in Sheet components with usePortal option.
- Refactor and fix image format validation on UI layer for better error handling.
- Implement organization avatar MongoDB model and error handling for improved data management.
- Fix front-end redirect issue to sign page and ledger typing on front-end for smoother user navigation.
- Adjust if statements and error messages for identity and auth for better error communication.

### 📚 Documentation
- Update console structure documentation and STRUCTURE.md with comprehensive architecture documentation for better developer guidance.

### 🔧 Maintenance
- Update packages and adjust package.json for dependency management.
- Merge code from old repo refactor for codebase consolidation.
- Clean up code and adjust naming consistency for improved code quality.
- Update components/console/src/lib/intl/use-format-number.ts for better internationalization support.

## [v2.2.0-beta.33] - 2025-05-23

### 🔧 Maintenance
- Update `github.com/gofiber/fiber` to the latest v2 version

## [v2.2.0-beta.32] - 2025-05-23

### 🐛 Bug Fixes
- Reduce cognitive complexity by extracting `handleAccountFields` and add `isConcat` for array manipulation, improving code readability and maintainability.

### 🔧 Maintenance
- Update CHANGELOG to reflect recent changes.

## [v2.2.0-beta.31] - 2025-05-22

### ✨ Features
- Add endpoint for outflow transactions to support external withdrawal operations

### 📚 Documentation
- Update CHANGELOG to reflect recent changes

## [v2.2.0-beta.30] - 2025-05-22

### ✨ Features
- Add Inflow Transaction Endpoint to support external funding operations

### 🔧 Maintenance
- Update HTTP adapter for transaction component
- Update Postgres adapter for transaction component

## [v2.2.0-beta.29] - 2025-05-21

### 🐛 Bug Fixes
- Prevent reinstalling golangci-lint if already installed

### 📚 Documentation
- Update CHANGELOG to reflect recent changes

## [v2.2.0-beta.28] - 2025-05-20

### 🐛 Bug Fixes
- Correct message format to align with `lib-commons` standards.
- Improve overflow handling for scales greater than 18 to prevent errors in mathematical operations.
- Add overflow validation to enhance stability in calculations.
- Remove auth network from OSS midaz onboarding and transaction processes to streamline user experience.

### 🔧 Maintenance
- Integrate `lib-commons` beta version, updating `go.mod` and `go.sum` for compatibility.
- Update `go.mod` and `go.sum` files to reflect the latest dependency versions.

### 📚 Documentation
- Update CHANGELOG to reflect recent changes and improvements.

## [v2.2.0-beta.27] - 2025-05-19

### ✨ Features
- Set up demo data for feature development, facilitating easier testing and demonstration of new functionalities (#812)

### 🔧 Maintenance
- Update CHANGELOG with recent changes

## [v2.2.0-beta.26] - 2025-05-17

### 🐛 Bug Fixes
- Correct console output in Makefile build script to ensure accurate logging during builds.

### 🔧 Maintenance
- Update `docker-compose.yml` configuration for improved setup and deployment processes. [#810]

## [v2.2.0-beta.25] - 2025-05-16

### ✨ Features
- Add support for retrieving balances by alias and external code, enhancing the flexibility of balance queries.

### 🐛 Bug Fixes
- Resolve linter issues to improve code quality and maintainability.
- Implement pagination in return values and update documentation to ensure efficient data handling and clarity in usage.
- Update log messages for clarity, enhancing the readability and usefulness of log outputs.


## [v2.2.0-beta.23] - 2025-05-14

### ✨ Features
- Add endpoint to retrieve external accounts by code

### 🐛 Bug Fixes
- Resolve linter issues to ensure code quality and adherence to standards

### 🔧 Maintenance
- Remove unused class to improve codebase maintainability

## [v2.2.0-beta.21] - 2025-05-14

### ✨ Features
- Support multiple transactions with the same account in From/To fields

### 🐛 Bug Fixes
- Rollback recent changes to stabilize the system
- Validate and handle invalid strings for account type, improving error handling
- Change error message to 'Invalid Account Type' for better user feedback

### 📚 Documentation
- Update account type field description for clarity

### 🔧 Maintenance
- Apply linting corrections to codebase

## [v2.2.0-beta.20] - 2025-05-13

### ✨ Features
- Add operation type filter to account operations for enhanced functionality, allowing users to refine their search and improve workflow efficiency.

### 🐛 Bug Fixes
- Update string formatting from '&' to '*' for improved clarity, ensuring consistent and clear presentation of information across the application.

### 🔧 Maintenance
- Update CHANGELOG to reflect recent changes, ensuring documentation is up-to-date and accurately represents the current state of the project.

## [v2.2.0-beta.19] - 2025-05-09

### 🐛 Bug Fixes
- Adjust test to improve reliability

### 🗑️ Removed
- ⚠️ **Breaking Change**: Remove transaction templates API and update MongoDB connection string

### 🔧 Maintenance
- Update CHANGELOG to reflect recent changes

## [v2.2.0-beta.18] - 2025-05-09

### 🔧 Maintenance
- Update CHANGELOG to reflect recent changes
- Bump Bubble Tea dependency from version 1.3.4 to 1.3.5

## [v2.2.0-beta.17] - 2025-05-09

### 📚 Documentation
- Update project description and features to reflect the latest changes

### 🔧 Maintenance
- Update CHANGELOG with the latest release information
- Bump `github.com/redis/go-redis/v9` from version 9.7.3 to 9.8.0 for improved performance and new features

## [v2.2.0-beta.16] - 2025-05-06

### 🔧 Maintenance
- Remove unused `APP_CONTEXT` environment variable to clean up configuration and improve code clarity.

### 📚 Documentation
- Update CHANGELOG to reflect recent changes and ensure accurate version tracking.

## [v2.2.0-beta.15] - 2025-05-05

### ✨ Features
- ⚠️ **Breaking Change**: Remove account type enum constraint, allowing more flexibility in account type definitions. This change may affect existing implementations relying on previous constraints.

### 🐛 Bug Fixes
- Map invalid account types correctly to ensure proper handling of unexpected inputs.
- Implement code review suggestions to enhance code quality and address minor issues identified during the review process.

### 📚 Documentation
- Update documentation for the account type field to reflect recent changes and improve clarity.

## [v2.2.0-beta.14] - 2025-05-02

### ✨ Features
- Clean temporary files before executing Goreleaser to ensure a clean build environment

## [v2.2.0-beta.12] - 2025-04-29

### ✨ Features
- Configure commit process with push bot application ID


## [v2.2.0-beta.11] - 2025-04-29

### ✨ Features
- Update `goreleaser` configuration to improve the release process, enhancing the efficiency and reliability of software releases.

### 🔧 Maintenance
- Ignore dirty files during `goreleaser` execution by using `git clean`, ensuring a clean working directory and preventing potential release issues.

## [2.2.0-beta.5](https://github.com/LerianStudio/midaz/compare/v2.2.0-beta.4...v2.2.0-beta.5) (2025-04-24)


### Bug Fixes

* improve transaction processing tracing, code quality and idempotency; ([fd377d9](https://github.com/LerianStudio/midaz/commit/fd377d9364e103dee5bcb8239810c7102b55137c))
* update go mod and go sum and update lib-auth method newauthclient with new parameter logger; ([13c751d](https://github.com/LerianStudio/midaz/commit/13c751d0e8b465da1ded90ff99d8c2c5f689d7cb))

## [2.2.0-beta.4](https://github.com/LerianStudio/midaz/compare/v2.2.0-beta.3...v2.2.0-beta.4) (2025-04-23)


### Bug Fixes

* **pipeline:** execute console integration tests in right directory during release workflow execution ([0c840d1](https://github.com/LerianStudio/midaz/commit/0c840d1999e0298b4edf3edcbc4c21acfb5638a4))

## [2.2.0-beta.3](https://github.com/LerianStudio/midaz/compare/v2.2.0-beta.2...v2.2.0-beta.3) (2025-04-17)

## [2.2.0-beta.2](https://github.com/LerianStudio/midaz/compare/v2.2.0-beta.1...v2.2.0-beta.2) (2025-04-15)


### Features

* Update workflows to use latest versions of dependencies and tools. ([6511248](https://github.com/LerianStudio/midaz/commit/6511248ef2b7a2c116c846675f96a8fb748d224b))


### Bug Fixes

* **pipeline:** enable pushing of Docker images in CI workflow ([ffeeb3b](https://github.com/LerianStudio/midaz/commit/ffeeb3b8b6274ce859c912c065eefb5e12fc1abe))
* **workflow:** update github-actions-changed-paths action to use main branch instead of develop ([fe08d78](https://github.com/LerianStudio/midaz/commit/fe08d789e31d558cec82bd496beb2d12b122d767))

## [2.2.0-beta.1](https://github.com/LerianStudio/midaz/compare/v2.1.0...v2.2.0-beta.1) (2025-04-10)


### Features

* **transaction:** adding accountAlias field to keep backward compatibility ([7c6875c](https://github.com/LerianStudio/midaz/commit/7c6875cf407da06456f5645390b61388f94c9a4b))
* define gorelease version on pipeline flow ([c845fe1](https://github.com/LerianStudio/midaz/commit/c845fe15ff5af0d562a554b44d6612820589208e))
* remove discord beta releases flow ([3e050eb](https://github.com/LerianStudio/midaz/commit/3e050eb863b694734c18ee32eb02547eec713056))
* **transaction:** removing deprecated message when account field is used insted accountAlias ([2e5a1ca](https://github.com/LerianStudio/midaz/commit/2e5a1ca0744362524bbf89fa1f189764a789d87f))
* **transaction:** removing deprecated message when account field is used insted accountAlias ([c674fde](https://github.com/LerianStudio/midaz/commit/c674fde681a6077af03462c59e094e5786aaa265))
* **transaction:** removing get-all-metadata-operations.go is not being used ([8a5014c](https://github.com/LerianStudio/midaz/commit/8a5014c0383cd0fddfc8f31625011d086723fb03))
* **transaction:** upgrading lib-commons to 1.5.0 versionwith new accountAlias field ([e6bb757](https://github.com/LerianStudio/midaz/commit/e6bb757a94c34ddfb39d3d3f2110daa565db7e1c))

## [2.1.0](https://github.com/LerianStudio/midaz/compare/v2.0.0...v2.1.0) (2025-04-08)


### Bug Fixes

* fixing import package ([82f51c7](https://github.com/LerianStudio/midaz/commit/82f51c7b773720dba2a1aea2b1d24f563eab652e))
* removing code comments ([21de234](https://github.com/LerianStudio/midaz/commit/21de23412f05de8a1943fc213eda8275bda998b1))
* upgrading dependencias ([1f6581f](https://github.com/LerianStudio/midaz/commit/1f6581f25e21d3bd8056e05f24cf4ab8cb9e5b8a))
* upgrading dependencias ([8511320](https://github.com/LerianStudio/midaz/commit/8511320727031a5eab6b522df25abde46f6eb368))
* use *bson.M instead of map[string]interface{} for metadata filter and unit testing ([21e63f1](https://github.com/LerianStudio/midaz/commit/21e63f152d97975b374dcbf9e83b619c8cc805c6))
* use *bson.M instead of map[string]interface{} for metadata filter and unit testing ([92a5d08](https://github.com/LerianStudio/midaz/commit/92a5d082dcfa93816fc166c435a2468c22fa01ac))

## [2.1.0-beta.2](https://github.com/LerianStudio/midaz/compare/v2.1.0-beta.1...v2.1.0-beta.2) (2025-04-08)

## [2.1.0-beta.1](https://github.com/LerianStudio/midaz/compare/v2.0.1-beta.2...v2.1.0-beta.1) (2025-04-08)


### Bug Fixes

* fixing import package ([82f51c7](https://github.com/LerianStudio/midaz/commit/82f51c7b773720dba2a1aea2b1d24f563eab652e))
* removing code comments ([21de234](https://github.com/LerianStudio/midaz/commit/21de23412f05de8a1943fc213eda8275bda998b1))
* upgrading dependencies ([1f6581f](https://github.com/LerianStudio/midaz/commit/1f6581f25e21d3bd8056e05f24cf4ab8cb9e5b8a))
* upgrading dependencies ([8511320](https://github.com/LerianStudio/midaz/commit/8511320727031a5eab6b522df25abde46f6eb368))
* metadata filter and unit testing ([21e63f1](https://github.com/LerianStudio/midaz/commit/21e63f152d97975b374dcbf9e83b619c8cc805c6))
* metadata filter and unit testing ([92a5d08](https://github.com/LerianStudio/midaz/commit/92a5d082dcfa93816fc166c435a2468c22fa01ac))

## [2.0.1-beta.2](https://github.com/LerianStudio/midaz/compare/v2.0.1-beta.1...v2.0.1-beta.2) (2025-04-08)

## [2.0.1-beta.1](https://github.com/LerianStudio/midaz/compare/v2.0.0...v2.0.1-beta.1) (2025-04-08)

## [2.0.0](https://github.com/LerianStudio/midaz/compare/v1.51.0...v2.0.0) (2025-04-05)


### ⚠ BREAKING CHANGES

* **release:** change

BREAKING

* feat(makefile): testing breaking change
* CHANGE

* feat(makefile): testing breaking change
* CHANGE
* **makefile:** CHANGE
* sync postman script auto installing dependencies
* **release:** change

BREAKING

* feat(makefile): testing breaking change
* CHANGE
* **makefile:** CHANGE
* sync postman script auto installing dependencies
* **release:** change

BREAKING
* makefile
* **release:** change

BREAKING

* feat(makefile): testing breaking change
* CHANGE

* feat(makefile): testing breaking change
* CHANGE
* **makefile:** CHANGE
* sync postman script auto installing dependencies
* **release:** change

BREAKING

* feat(makefile): testing breaking change
* CHANGE
* **makefile:** CHANGE
* sync postman script auto installing dependencies
* **release:** change

BREAKING
* sync postman script auto installing dependencies
* breaking change

BREAKING

* feat(makefile): testing breaking change
* CHANGE

* feat(makefile): testing breaking change
* CHANGE
* **makefile:** CHANGE
* sync postman script auto installing dependencies
* breaking change

BREAKING

* feat(makefile): testing breaking change
* CHANGE
* **makefile:** CHANGE
* sync postman script auto installing dependencies
* breaking change

BREAKING

* Merge pull request #666 from LerianStudio/fix/pumping-3 ([f8ea3ea](https://github.com/LerianStudio/midaz/commit/f8ea3eade70044f2810c2e22587883cd00a83430)), closes [#666](https://github.com/LerianStudio/midaz/issues/666)
* Breaking/installing node on sync postman process (#650) ([659c0fb](https://github.com/LerianStudio/midaz/commit/659c0fb6c4f2760323993afb35381308b9c5a498)), closes [#650](https://github.com/LerianStudio/midaz/issues/650)
* Breaking/installing node on sync postman process (#649) ([4ec2d1b](https://github.com/LerianStudio/midaz/commit/4ec2d1b2ea896c458877cb9e1bd2cff7185ded9f)), closes [#649](https://github.com/LerianStudio/midaz/issues/649)
* Breaking/installing node on sync postman process (#648) ([42b8dac](https://github.com/LerianStudio/midaz/commit/42b8dac80a2287e9b52a5b0c793349f9c471a91a)), closes [#648](https://github.com/LerianStudio/midaz/issues/648)


### Features

* add entity_id optional on post and add on patch to update; :sparkles: ([405cab3](https://github.com/LerianStudio/midaz/commit/405cab3cfa3733c6658208e65b5e2c88ef7021ad))
* adding plugin auth network into midaz ([3ad0a6f](https://github.com/LerianStudio/midaz/commit/3ad0a6f62236dab02b1a99648de2e12c68147152))
* adjust mongo.sh to init configs; :sparkles: ([8104d3d](https://github.com/LerianStudio/midaz/commit/8104d3debce27b4a6cd675113da0dac134b333ef))
* enable logical replication in Postgres and configure MongoDB replica set; :sparkles: ([93e14e9](https://github.com/LerianStudio/midaz/commit/93e14e9af47c4d44ba158336084c9ed5dfc10758))
* increase checkout for v4 ([ee9d982](https://github.com/LerianStudio/midaz/commit/ee9d982059222a8373883e0aac1f91df6d5d9660))
* mantain the name of jobs on Midaz ([67c34b3](https://github.com/LerianStudio/midaz/commit/67c34b3d543f0dc54aef1d5249c0072db1edc029))
* migrate golangci-lint to v2 on pipeline validations ([6fa1dc5](https://github.com/LerianStudio/midaz/commit/6fa1dc57498f081c8d4c32d5352b130627774ee3))
* organize golangci-lint on module ([d9735f8](https://github.com/LerianStudio/midaz/commit/d9735f8eaf2022188a9d0b63dd182afc8e0cce60))
* **makefile:** testing breaking change ([1791699](https://github.com/LerianStudio/midaz/commit/17916994923c7ec302d2277598a76903b952c1dd))
* **makefile:** testing breaking change ([106af7c](https://github.com/LerianStudio/midaz/commit/106af7c21c8abe1f007dafbfe2cae1410bdbf547))
* update libs on go.mod and go.sum; :sparkles: ([1b04822](https://github.com/LerianStudio/midaz/commit/1b04822c3309fda6c3094988fff3f616bd23d46a))


### Bug Fixes

* add error response from lib-commons to return right business error; :bug: ([27af975](https://github.com/LerianStudio/midaz/commit/27af97542a36d619dca0e04bd6039c195701650e))
* add omitempty to avoid nested erro when metadata receives null; :bug: ([4dbaa6f](https://github.com/LerianStudio/midaz/commit/4dbaa6f4db2419caa35f26d66726823831feb54b))
* add right return erros and status codes; :bug: ([02791d5](https://github.com/LerianStudio/midaz/commit/02791d5cb29649c6f7b6a7714cad58383bc63e69))
* **metadata:** add support for updating or removing metadata using JSON Merge Patch; :bug: ([18d2315](https://github.com/LerianStudio/midaz/commit/18d2315a267727066d0500ad5a56e3ca0a784f0b))
* adjust other places that we change pkg error for lib-commons error; :bug: ([5b07da5](https://github.com/LerianStudio/midaz/commit/5b07da53c91cc006a152211e360bd1d18b11464c))
* adjust to return right error and status code; :bug: ([7aaca78](https://github.com/LerianStudio/midaz/commit/7aaca782afb65de385f22b0eadb1dbc6c373efcd))
* adjust to return right error code and status; :bug: ([a887f4f](https://github.com/LerianStudio/midaz/commit/a887f4f6fb876dcf0f7e5bfeec968e0fc9ac3b3d))
* change return error 400 to 404 when find account by alias; :bug: ([7a4bb31](https://github.com/LerianStudio/midaz/commit/7a4bb31aa84b4c0da73f90462d41d9dce1d3c895))
* http error code data range and sort fields; :bug: ([b267c47](https://github.com/LerianStudio/midaz/commit/b267c47d5b1a2e4b35870ed4602d9704f8ee110c))
* improve json unmarshal error handling with detailed field feedback :bug: ([549d5e0](https://github.com/LerianStudio/midaz/commit/549d5e06be8c819815fea8f5b736fdd965aa6297))
* rabbitmq mispelling name; :bug: ([55e2525](https://github.com/LerianStudio/midaz/commit/55e252565db2675c79a43f9b30f359d3a2d99d7b))
* update go mod and go sum; :bug: ([0dcd926](https://github.com/LerianStudio/midaz/commit/0dcd92605116ea4c8df9e094a83cff57b8f43136))
* update go mod and sum; :bug: ([0648e2d](https://github.com/LerianStudio/midaz/commit/0648e2d52208205ea978045431ad3750eb9c536d))
* update postman add api find by alias; :bug: ([46fcc3c](https://github.com/LerianStudio/midaz/commit/46fcc3c34c76901df34b9ac8ef76822184ccfbf3))
* update Swagger documentation generation process ([#606](https://github.com/LerianStudio/midaz/issues/606)) ([2cca7a2](https://github.com/LerianStudio/midaz/commit/2cca7a2c3124a3e7c46bf7f642506051e94ba112))
* use default variable in channel qos; set golangci lint version; :bug: ([a15a80f](https://github.com/LerianStudio/midaz/commit/a15a80fdf2d26cba1eef603d6a74de19de9aeb94))


### Miscellaneous Chores

* **release:** 1.51.0-beta.18 ([5c04a37](https://github.com/LerianStudio/midaz/commit/5c04a378a5becc287281cda040775de14cbe4fea)), closes [#650](https://github.com/LerianStudio/midaz/issues/650) [#649](https://github.com/LerianStudio/midaz/issues/649) [#648](https://github.com/LerianStudio/midaz/issues/648)
* **release:** 2.0.0-beta.1 ([1a79ccd](https://github.com/LerianStudio/midaz/commit/1a79ccd9171bc095463961e1e5c57dec7d771413)), closes [#666](https://github.com/LerianStudio/midaz/issues/666) [#650](https://github.com/LerianStudio/midaz/issues/650) [#649](https://github.com/LerianStudio/midaz/issues/649) [#648](https://github.com/LerianStudio/midaz/issues/648) [#606](https://github.com/LerianStudio/midaz/issues/606) [#650](https://github.com/LerianStudio/midaz/issues/650) [#649](https://github.com/LerianStudio/midaz/issues/649) [#648](https://github.com/LerianStudio/midaz/issues/648)

## [2.0.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.51.0...v2.0.0-beta.1) (2025-04-05)


### ⚠ BREAKING CHANGES

* makefile
* **release:** change

BREAKING

* feat(makefile): testing breaking change
* CHANGE

* feat(makefile): testing breaking change
* CHANGE
* **makefile:** CHANGE
* sync postman script auto installing dependencies
* **release:** change

BREAKING

* feat(makefile): testing breaking change
* CHANGE
* **makefile:** CHANGE
* sync postman script auto installing dependencies
* **release:** change

BREAKING
* sync postman script auto installing dependencies
* breaking change

BREAKING

* feat(makefile): testing breaking change
* CHANGE

* feat(makefile): testing breaking change
* CHANGE
* **makefile:** CHANGE
* sync postman script auto installing dependencies
* breaking change

BREAKING

* feat(makefile): testing breaking change
* CHANGE
* **makefile:** CHANGE
* sync postman script auto installing dependencies
* breaking change

BREAKING

* Merge pull request #666 from LerianStudio/fix/pumping-3 ([f8ea3ea](https://github.com/LerianStudio/midaz/commit/f8ea3eade70044f2810c2e22587883cd00a83430)), closes [#666](https://github.com/LerianStudio/midaz/issues/666)
* Breaking/installing node on sync postman process (#650) ([659c0fb](https://github.com/LerianStudio/midaz/commit/659c0fb6c4f2760323993afb35381308b9c5a498)), closes [#650](https://github.com/LerianStudio/midaz/issues/650)
* Breaking/installing node on sync postman process (#649) ([4ec2d1b](https://github.com/LerianStudio/midaz/commit/4ec2d1b2ea896c458877cb9e1bd2cff7185ded9f)), closes [#649](https://github.com/LerianStudio/midaz/issues/649)
* Breaking/installing node on sync postman process (#648) ([42b8dac](https://github.com/LerianStudio/midaz/commit/42b8dac80a2287e9b52a5b0c793349f9c471a91a)), closes [#648](https://github.com/LerianStudio/midaz/issues/648)


### Features

* add entity_id optional on post and add on patch to update; :sparkles: ([405cab3](https://github.com/LerianStudio/midaz/commit/405cab3cfa3733c6658208e65b5e2c88ef7021ad))
* adding plugin auth network into midaz ([3ad0a6f](https://github.com/LerianStudio/midaz/commit/3ad0a6f62236dab02b1a99648de2e12c68147152))
* adjust mongo.sh to init configs; :sparkles: ([8104d3d](https://github.com/LerianStudio/midaz/commit/8104d3debce27b4a6cd675113da0dac134b333ef))
* enable logical replication in Postgres and configure MongoDB replica set; :sparkles: ([93e14e9](https://github.com/LerianStudio/midaz/commit/93e14e9af47c4d44ba158336084c9ed5dfc10758))
* increase checkout for v4 ([ee9d982](https://github.com/LerianStudio/midaz/commit/ee9d982059222a8373883e0aac1f91df6d5d9660))
* mantain the name of jobs on Midaz ([67c34b3](https://github.com/LerianStudio/midaz/commit/67c34b3d543f0dc54aef1d5249c0072db1edc029))
* migrate golangci-lint to v2 on pipeline validations ([6fa1dc5](https://github.com/LerianStudio/midaz/commit/6fa1dc57498f081c8d4c32d5352b130627774ee3))
* organize golangci-lint on module ([d9735f8](https://github.com/LerianStudio/midaz/commit/d9735f8eaf2022188a9d0b63dd182afc8e0cce60))
* **makefile:** testing breaking change ([1791699](https://github.com/LerianStudio/midaz/commit/17916994923c7ec302d2277598a76903b952c1dd))
* **makefile:** testing breaking change ([106af7c](https://github.com/LerianStudio/midaz/commit/106af7c21c8abe1f007dafbfe2cae1410bdbf547))
* update libs on go.mod and go.sum; :sparkles: ([1b04822](https://github.com/LerianStudio/midaz/commit/1b04822c3309fda6c3094988fff3f616bd23d46a))


### Bug Fixes

* add error response from lib-commons to return right business error; :bug: ([27af975](https://github.com/LerianStudio/midaz/commit/27af97542a36d619dca0e04bd6039c195701650e))
* add omitempty to avoid nested erro when metadata receives null; :bug: ([4dbaa6f](https://github.com/LerianStudio/midaz/commit/4dbaa6f4db2419caa35f26d66726823831feb54b))
* add right return erros and status codes; :bug: ([02791d5](https://github.com/LerianStudio/midaz/commit/02791d5cb29649c6f7b6a7714cad58383bc63e69))
* **metadata:** add support for updating or removing metadata using JSON Merge Patch; :bug: ([18d2315](https://github.com/LerianStudio/midaz/commit/18d2315a267727066d0500ad5a56e3ca0a784f0b))
* adjust other places that we change pkg error for lib-commons error; :bug: ([5b07da5](https://github.com/LerianStudio/midaz/commit/5b07da53c91cc006a152211e360bd1d18b11464c))
* adjust to return right error and status code; :bug: ([7aaca78](https://github.com/LerianStudio/midaz/commit/7aaca782afb65de385f22b0eadb1dbc6c373efcd))
* adjust to return right error code and status; :bug: ([a887f4f](https://github.com/LerianStudio/midaz/commit/a887f4f6fb876dcf0f7e5bfeec968e0fc9ac3b3d))
* change return error 400 to 404 when find account by alias; :bug: ([7a4bb31](https://github.com/LerianStudio/midaz/commit/7a4bb31aa84b4c0da73f90462d41d9dce1d3c895))
* http error code data range and sort fields; :bug: ([b267c47](https://github.com/LerianStudio/midaz/commit/b267c47d5b1a2e4b35870ed4602d9704f8ee110c))
* improve json unmarshal error handling with detailed field feedback :bug: ([549d5e0](https://github.com/LerianStudio/midaz/commit/549d5e06be8c819815fea8f5b736fdd965aa6297))
* rabbitmq mispelling name; :bug: ([55e2525](https://github.com/LerianStudio/midaz/commit/55e252565db2675c79a43f9b30f359d3a2d99d7b))
* update go mod and go sum; :bug: ([0dcd926](https://github.com/LerianStudio/midaz/commit/0dcd92605116ea4c8df9e094a83cff57b8f43136))
* update go mod and sum; :bug: ([0648e2d](https://github.com/LerianStudio/midaz/commit/0648e2d52208205ea978045431ad3750eb9c536d))
* update postman add api find by alias; :bug: ([46fcc3c](https://github.com/LerianStudio/midaz/commit/46fcc3c34c76901df34b9ac8ef76822184ccfbf3))
* update Swagger documentation generation process ([#606](https://github.com/LerianStudio/midaz/issues/606)) ([2cca7a2](https://github.com/LerianStudio/midaz/commit/2cca7a2c3124a3e7c46bf7f642506051e94ba112))
* use default variable in channel qos; set golangci lint version; :bug: ([a15a80f](https://github.com/LerianStudio/midaz/commit/a15a80fdf2d26cba1eef603d6a74de19de9aeb94))


### Miscellaneous Chores

* **release:** 1.51.0-beta.18 ([5c04a37](https://github.com/LerianStudio/midaz/commit/5c04a378a5becc287281cda040775de14cbe4fea)), closes [#650](https://github.com/LerianStudio/midaz/issues/650) [#649](https://github.com/LerianStudio/midaz/issues/649) [#648](https://github.com/LerianStudio/midaz/issues/648)

## [2.0.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.51.0-beta.19...v2.0.0-beta.1) (2025-04-05)

## [2.0.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.51.0-beta.19...v2.0.0-beta.1) (2025-04-04)

## [1.51.0](https://github.com/LerianStudio/midaz/compare/v1.50.0...v1.51.0) (2025-04-04)


### Bug Fixes

* remove old description about midaz on readme; :bug: ([160fb34](https://github.com/LerianStudio/midaz/commit/160fb348abbea1a5969f50c12723a1675eb10772))

## [1.51.0-beta.17](https://github.com/LerianStudio/midaz/compare/v1.51.0-beta.16...v1.51.0-beta.17) (2025-04-04)


### Bug Fixes

* add right return erros and status codes; :bug: ([02791d5](https://github.com/LerianStudio/midaz/commit/02791d5cb29649c6f7b6a7714cad58383bc63e69))
* rabbitmq mispelling name; :bug: ([55e2525](https://github.com/LerianStudio/midaz/commit/55e252565db2675c79a43f9b30f359d3a2d99d7b))

## [1.51.0-beta.16](https://github.com/LerianStudio/midaz/compare/v1.51.0-beta.15...v1.51.0-beta.16) (2025-04-03)


### Bug Fixes

* improve json unmarshal error handling with detailed field feedback :bug: ([549d5e0](https://github.com/LerianStudio/midaz/commit/549d5e06be8c819815fea8f5b736fdd965aa6297))

## [1.51.0-beta.15](https://github.com/LerianStudio/midaz/compare/v1.51.0-beta.14...v1.51.0-beta.15) (2025-04-03)


### Bug Fixes

* adjust to return right error code and status; :bug: ([a887f4f](https://github.com/LerianStudio/midaz/commit/a887f4f6fb876dcf0f7e5bfeec968e0fc9ac3b3d))
* update go mod and go sum; :bug: ([0dcd926](https://github.com/LerianStudio/midaz/commit/0dcd92605116ea4c8df9e094a83cff57b8f43136))
* update go mod and sum; :bug: ([0648e2d](https://github.com/LerianStudio/midaz/commit/0648e2d52208205ea978045431ad3750eb9c536d))

## [1.51.0-beta.14](https://github.com/LerianStudio/midaz/compare/v1.51.0-beta.13...v1.51.0-beta.14) (2025-04-03)


### Bug Fixes

* change return error 400 to 404 when find account by alias; :bug: ([7a4bb31](https://github.com/LerianStudio/midaz/commit/7a4bb31aa84b4c0da73f90462d41d9dce1d3c895))
* update postman add api find by alias; :bug: ([46fcc3c](https://github.com/LerianStudio/midaz/commit/46fcc3c34c76901df34b9ac8ef76822184ccfbf3))

## [1.51.0-beta.13](https://github.com/LerianStudio/midaz/compare/v1.51.0-beta.12...v1.51.0-beta.13) (2025-04-03)


### Bug Fixes

* adjust to return right error and status code; :bug: ([7aaca78](https://github.com/LerianStudio/midaz/commit/7aaca782afb65de385f22b0eadb1dbc6c373efcd))

## [1.51.0-beta.12](https://github.com/LerianStudio/midaz/compare/v1.51.0-beta.11...v1.51.0-beta.12) (2025-04-02)


### Features

* add entity_id optional on post and add on patch to update; :sparkles: ([405cab3](https://github.com/LerianStudio/midaz/commit/405cab3cfa3733c6658208e65b5e2c88ef7021ad))
* migrate golangci-lint to v2 on pipeline validations ([6fa1dc5](https://github.com/LerianStudio/midaz/commit/6fa1dc57498f081c8d4c32d5352b130627774ee3))

## [1.51.0-beta.11](https://github.com/LerianStudio/midaz/compare/v1.51.0-beta.10...v1.51.0-beta.11) (2025-04-02)


### Bug Fixes

* http error code data range and sort fields; :bug: ([b267c47](https://github.com/LerianStudio/midaz/commit/b267c47d5b1a2e4b35870ed4602d9704f8ee110c))

## [1.51.0-beta.10](https://github.com/LerianStudio/midaz/compare/v1.51.0-beta.9...v1.51.0-beta.10) (2025-04-01)


### Features

* adding plugin auth network into midaz ([3ad0a6f](https://github.com/LerianStudio/midaz/commit/3ad0a6f62236dab02b1a99648de2e12c68147152))

## [1.51.0-beta.9](https://github.com/LerianStudio/midaz/compare/v1.51.0-beta.8...v1.51.0-beta.9) (2025-04-01)


### Features

* update libs on go.mod and go.sum; :sparkles: ([1b04822](https://github.com/LerianStudio/midaz/commit/1b04822c3309fda6c3094988fff3f616bd23d46a))

## [1.51.0-beta.8](https://github.com/LerianStudio/midaz/compare/v1.51.0-beta.7...v1.51.0-beta.8) (2025-04-01)

## [1.51.0-beta.7](https://github.com/LerianStudio/midaz/compare/v1.51.0-beta.6...v1.51.0-beta.7) (2025-03-31)


### Bug Fixes

* **metadata:** add support for updating or removing metadata using JSON Merge Patch; :bug: ([18d2315](https://github.com/LerianStudio/midaz/commit/18d2315a267727066d0500ad5a56e3ca0a784f0b))

## [1.51.0-beta.6](https://github.com/LerianStudio/midaz/compare/v1.51.0-beta.5...v1.51.0-beta.6) (2025-03-31)


### Bug Fixes

* add omitempty to avoid nested erro when metadata receives null; :bug: ([4dbaa6f](https://github.com/LerianStudio/midaz/commit/4dbaa6f4db2419caa35f26d66726823831feb54b))

## [1.51.0-beta.5](https://github.com/LerianStudio/midaz/compare/v1.51.0-beta.4...v1.51.0-beta.5) (2025-03-31)


### Bug Fixes

* update Swagger documentation generation process ([#606](https://github.com/LerianStudio/midaz/issues/606)) ([2cca7a2](https://github.com/LerianStudio/midaz/commit/2cca7a2c3124a3e7c46bf7f642506051e94ba112))

## [1.51.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.51.0-beta.3...v1.51.0-beta.4) (2025-03-27)


### Features

* adjust mongo.sh to init configs; :sparkles: ([8104d3d](https://github.com/LerianStudio/midaz/commit/8104d3debce27b4a6cd675113da0dac134b333ef))
* enable logical replication in Postgres and configure MongoDB replica set; :sparkles: ([93e14e9](https://github.com/LerianStudio/midaz/commit/93e14e9af47c4d44ba158336084c9ed5dfc10758))

## [1.51.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.51.0-beta.2...v1.51.0-beta.3) (2025-03-27)


### Features

* increase checkout for v4 ([ee9d982](https://github.com/LerianStudio/midaz/commit/ee9d982059222a8373883e0aac1f91df6d5d9660))
* mantain the name of jobs on Midaz ([67c34b3](https://github.com/LerianStudio/midaz/commit/67c34b3d543f0dc54aef1d5249c0072db1edc029))
* organize golangci-lint on module ([d9735f8](https://github.com/LerianStudio/midaz/commit/d9735f8eaf2022188a9d0b63dd182afc8e0cce60))

## [1.51.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.51.0-beta.1...v1.51.0-beta.2) (2025-03-26)


### Bug Fixes

* add error response from lib-commons to return right business error; :bug: ([27af975](https://github.com/LerianStudio/midaz/commit/27af97542a36d619dca0e04bd6039c195701650e))
* adjust other places that we change pkg error for lib-commons error; :bug: ([5b07da5](https://github.com/LerianStudio/midaz/commit/5b07da53c91cc006a152211e360bd1d18b11464c))

## [1.51.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.50.0...v1.51.0-beta.1) (2025-03-24)


### Bug Fixes

* remove old description about midaz on readme; :bug: ([160fb34](https://github.com/LerianStudio/midaz/commit/160fb348abbea1a5969f50c12723a1675eb10772))
* use default variable in channel qos; set golangci lint version; :bug: ([a15a80f](https://github.com/LerianStudio/midaz/commit/a15a80fdf2d26cba1eef603d6a74de19de9aeb94))

## [1.50.0](https://github.com/LerianStudio/midaz/compare/v1.49.0...v1.50.0) (2025-03-21)


### Features

* add variable number of workers to env for modify anytime; ([3a4f707](https://github.com/LerianStudio/midaz/commit/3a4f707d901893f1948f740ea22c36b46e153ab2))
* implement new fast way to improve balance update values; :sparkles: ([f64195a](https://github.com/LerianStudio/midaz/commit/f64195a1c499571a367cc711ea726c442ef5193c))


### Bug Fixes

* add indexes and reindex commands for balance table :bug: ([d579867](https://github.com/LerianStudio/midaz/commit/d57986753a9ba9c0e11275abee0047a1b8af25f3))
* add metadata save on mongodb; rollback migrations on postgresql; :bug: ([7c726dc](https://github.com/LerianStudio/midaz/commit/7c726dcdec3dacddc78e8d34e23d97853c466ad9))
* add more index in account to improve performance; :bug: ([1aff75d](https://github.com/LerianStudio/midaz/commit/1aff75d65599e98c62b2a1877e268f99a7434b02))
* add networking changes from branch refactor/networks :bug: ([c4e10c9](https://github.com/LerianStudio/midaz/commit/c4e10c9f4d3ee638538446b19207ec2f3b108082))
* add new index; :bug: ([8bfb1dd](https://github.com/LerianStudio/midaz/commit/8bfb1ddd9620cd37df99c191eae87321e4f0cad7))
* add right ValidateBusinessError from pkg; :bug: ([4f74677](https://github.com/LerianStudio/midaz/commit/4f746771b5dfe8a62fd319bbf904a6b0b37e4d7c))
* adding plugin auth network as external network ([e4b135c](https://github.com/LerianStudio/midaz/commit/e4b135c737c5da6b6438a5c689647a22ea11c15d))
* adjust balance update to not return err when don't have rows affected; :bug: ([53c64c6](https://github.com/LerianStudio/midaz/commit/53c64c655888107fd6834cfe011488b826440c7c))
* adjust lib-auth to use method instead of struct; :bug: ([ff31389](https://github.com/LerianStudio/midaz/commit/ff3138964187a70801f49c52fd147b4696e96e30))
* adjusts tests and change lib-auth; :bug: ([a317bbe](https://github.com/LerianStudio/midaz/commit/a317bbe6fcd70c0cc4d6964d75c51fc2044a0782))
* changing pagination limit error type to validation error ([1e19108](https://github.com/LerianStudio/midaz/commit/1e19108af5394299469e74cf1ae7d4dbde78e4be))
* empty spaces; :bug: ([156e231](https://github.com/LerianStudio/midaz/commit/156e23126ff6eeeea87656ca366848e96cc12275))
* handle migration errors gracefully :bug: ([e464a75](https://github.com/LerianStudio/midaz/commit/e464a75875f02ae0ccd9d3fb4b0f6a1bd4580fd4))
* lint; :bug: ([1b0b7b5](https://github.com/LerianStudio/midaz/commit/1b0b7b5e2a3e544f4c174b2d230104edd275f214))
* lint; :bug: ([dea31c0](https://github.com/LerianStudio/midaz/commit/dea31c0e21d23e98459209b69ef1ca5406a5b301))
* lint; :bug: ([cf83e14](https://github.com/LerianStudio/midaz/commit/cf83e14d3ef09dea1aec0f80d4280725447ed0ea))
* lint; :bug: ([4b21161](https://github.com/LerianStudio/midaz/commit/4b21161b8bfe4e98d4b09616475070b8ca721d02))
* remove old reference from ledger grpc error message; :bug: ([1d0e762](https://github.com/LerianStudio/midaz/commit/1d0e762ec5bb325b6307397dafe4fcb86b82d7a4))
* remove pgbouncer; :bug: ([ca05b31](https://github.com/LerianStudio/midaz/commit/ca05b31518c66dd8ed7d8741c5f787d278872965))
* remove plugin network; :bug: ([3b3519c](https://github.com/LerianStudio/midaz/commit/3b3519ca9d214c84ffd030036b34db39f742f6ae))
* resotred workflow trigger to original ([17e88db](https://github.com/LerianStudio/midaz/commit/17e88db5c42bd165b3799b9c1516eb21c80a14a1))
* restored workflow steps ([8a4d3f7](https://github.com/LerianStudio/midaz/commit/8a4d3f7999bb1e91ff8a74bde40bb1005b1edbb1))
* return select for update for check a update balance method; :bug: ([d857237](https://github.com/LerianStudio/midaz/commit/d857237140657e48a9c83db6b8f3b3ee94d73b8c))
* rollback deploy tag on docker-compose; :bug: ([47c24da](https://github.com/LerianStudio/midaz/commit/47c24dafccb0d4eeb672d46e3f0195789e5ff32b))
* run workflow on branch push to test ([c0fb3bf](https://github.com/LerianStudio/midaz/commit/c0fb3bf44c6d5db9facf2515d521517ff9587682))
* some wrong variables references; :bug: ([8ed8750](https://github.com/LerianStudio/midaz/commit/8ed875043702869af09ba14e000f8acfc08da94a))
* trying to comment users on discord release channel ([2d37f05](https://github.com/LerianStudio/midaz/commit/2d37f05fb45616b94e878c68ae410abb72d3461f))
* unit tests; :bug: ([6742706](https://github.com/LerianStudio/midaz/commit/67427063505dafc61542f79b129343205f9bc451))
* update all gets to using deleted_at is null; :bug: ([4fd4940](https://github.com/LerianStudio/midaz/commit/4fd4940e2cdd0a7987c50441215c529cad772fba))
* update Discord webhook action and comment mentioning format ([5296b86](https://github.com/LerianStudio/midaz/commit/5296b8602cd02b7133def0f87a157aabaad1c1cf))
* update migrations error when don't have files on dir; :bug: ([fd629ad](https://github.com/LerianStudio/midaz/commit/fd629adf9a042ad820fdf1e36e5485faf32033a0))
* update output format for release notification workflow ([b50b79f](https://github.com/LerianStudio/midaz/commit/b50b79f5eba2cd2dbefc5aa65d910ab7858aae9d))
* update release notification workflow for Discord integration ([9ad5597](https://github.com/LerianStudio/midaz/commit/9ad5597351b7c2b35812b3ab179dbce809818f63))

## [1.50.0-beta.5](https://github.com/LerianStudio/midaz/compare/v1.50.0-beta.4...v1.50.0-beta.5) (2025-03-21)


### Features

* add variable number of workers to env for modify anytime; ([3a4f707](https://github.com/LerianStudio/midaz/commit/3a4f707d901893f1948f740ea22c36b46e153ab2))


### Bug Fixes

* add networking changes from branch refactor/networks :bug: ([c4e10c9](https://github.com/LerianStudio/midaz/commit/c4e10c9f4d3ee638538446b19207ec2f3b108082))
* add right ValidateBusinessError from pkg; :bug: ([4f74677](https://github.com/LerianStudio/midaz/commit/4f746771b5dfe8a62fd319bbf904a6b0b37e4d7c))
* adding plugin auth network as external network ([e4b135c](https://github.com/LerianStudio/midaz/commit/e4b135c737c5da6b6438a5c689647a22ea11c15d))
* adjust balance update to not return err when don't have rows affected; :bug: ([53c64c6](https://github.com/LerianStudio/midaz/commit/53c64c655888107fd6834cfe011488b826440c7c))
* adjust lib-auth to use method instead of struct; :bug: ([ff31389](https://github.com/LerianStudio/midaz/commit/ff3138964187a70801f49c52fd147b4696e96e30))
* adjusts tests and change lib-auth; :bug: ([a317bbe](https://github.com/LerianStudio/midaz/commit/a317bbe6fcd70c0cc4d6964d75c51fc2044a0782))
* empty spaces; :bug: ([156e231](https://github.com/LerianStudio/midaz/commit/156e23126ff6eeeea87656ca366848e96cc12275))
* remove plugin network; :bug: ([3b3519c](https://github.com/LerianStudio/midaz/commit/3b3519ca9d214c84ffd030036b34db39f742f6ae))
* return select for update for check a update balance method; :bug: ([d857237](https://github.com/LerianStudio/midaz/commit/d857237140657e48a9c83db6b8f3b3ee94d73b8c))
* rollback deploy tag on docker-compose; :bug: ([47c24da](https://github.com/LerianStudio/midaz/commit/47c24dafccb0d4eeb672d46e3f0195789e5ff32b))

## [1.50.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.50.0-beta.3...v1.50.0-beta.4) (2025-03-21)


### Bug Fixes

* changing pagination limit error type to validation error ([1e19108](https://github.com/LerianStudio/midaz/commit/1e19108af5394299469e74cf1ae7d4dbde78e4be))

## [1.50.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.50.0-beta.2...v1.50.0-beta.3) (2025-03-12)


### Bug Fixes

* add indexes and reindex commands for balance table :bug: ([d579867](https://github.com/LerianStudio/midaz/commit/d57986753a9ba9c0e11275abee0047a1b8af25f3))
* add metadata save on mongodb; rollback migrations on postgresql; :bug: ([7c726dc](https://github.com/LerianStudio/midaz/commit/7c726dcdec3dacddc78e8d34e23d97853c466ad9))
* add more index in account to improve performance; :bug: ([1aff75d](https://github.com/LerianStudio/midaz/commit/1aff75d65599e98c62b2a1877e268f99a7434b02))
* add new index; :bug: ([8bfb1dd](https://github.com/LerianStudio/midaz/commit/8bfb1ddd9620cd37df99c191eae87321e4f0cad7))
* handle migration errors gracefully :bug: ([e464a75](https://github.com/LerianStudio/midaz/commit/e464a75875f02ae0ccd9d3fb4b0f6a1bd4580fd4))
* remove old reference from ledger grpc error message; :bug: ([1d0e762](https://github.com/LerianStudio/midaz/commit/1d0e762ec5bb325b6307397dafe4fcb86b82d7a4))
* remove pgbouncer; :bug: ([ca05b31](https://github.com/LerianStudio/midaz/commit/ca05b31518c66dd8ed7d8741c5f787d278872965))
* some wrong variables references; :bug: ([8ed8750](https://github.com/LerianStudio/midaz/commit/8ed875043702869af09ba14e000f8acfc08da94a))
* unit tests; :bug: ([6742706](https://github.com/LerianStudio/midaz/commit/67427063505dafc61542f79b129343205f9bc451))
* update all gets to using deleted_at is null; :bug: ([4fd4940](https://github.com/LerianStudio/midaz/commit/4fd4940e2cdd0a7987c50441215c529cad772fba))

## [1.50.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.50.0-beta.1...v1.50.0-beta.2) (2025-03-11)


### Bug Fixes

* resotred workflow trigger to original ([17e88db](https://github.com/LerianStudio/midaz/commit/17e88db5c42bd165b3799b9c1516eb21c80a14a1))
* restored workflow steps ([8a4d3f7](https://github.com/LerianStudio/midaz/commit/8a4d3f7999bb1e91ff8a74bde40bb1005b1edbb1))
* run workflow on branch push to test ([c0fb3bf](https://github.com/LerianStudio/midaz/commit/c0fb3bf44c6d5db9facf2515d521517ff9587682))
* trying to comment users on discord release channel ([2d37f05](https://github.com/LerianStudio/midaz/commit/2d37f05fb45616b94e878c68ae410abb72d3461f))
* update Discord webhook action and comment mentioning format ([5296b86](https://github.com/LerianStudio/midaz/commit/5296b8602cd02b7133def0f87a157aabaad1c1cf))
* update output format for release notification workflow ([b50b79f](https://github.com/LerianStudio/midaz/commit/b50b79f5eba2cd2dbefc5aa65d910ab7858aae9d))
* update release notification workflow for Discord integration ([9ad5597](https://github.com/LerianStudio/midaz/commit/9ad5597351b7c2b35812b3ab179dbce809818f63))

## [1.50.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.49.0...v1.50.0-beta.1) (2025-03-10)


### Features

* implement new fast way to improve balance update values; :sparkles: ([f64195a](https://github.com/LerianStudio/midaz/commit/f64195a1c499571a367cc711ea726c442ef5193c))


### Bug Fixes

* lint; :bug: ([1b0b7b5](https://github.com/LerianStudio/midaz/commit/1b0b7b5e2a3e544f4c174b2d230104edd275f214))
* lint; :bug: ([dea31c0](https://github.com/LerianStudio/midaz/commit/dea31c0e21d23e98459209b69ef1ca5406a5b301))
* lint; :bug: ([cf83e14](https://github.com/LerianStudio/midaz/commit/cf83e14d3ef09dea1aec0f80d4280725447ed0ea))
* lint; :bug: ([4b21161](https://github.com/LerianStudio/midaz/commit/4b21161b8bfe4e98d4b09616475070b8ca721d02))
* update migrations error when don't have files on dir; :bug: ([fd629ad](https://github.com/LerianStudio/midaz/commit/fd629adf9a042ad820fdf1e36e5485faf32033a0))

## [1.49.0](https://github.com/LerianStudio/midaz/compare/v1.48.0...v1.49.0) (2025-03-07)


### Features

* add 5 works to consumer in rabbitmq and implement message persistent; ([1625d3b](https://github.com/LerianStudio/midaz/commit/1625d3b4aa22ad0889abdd23c97fa41e17714668))
* add a validations to cannot delete balance with funds; :sparkles: ([047de68](https://github.com/LerianStudio/midaz/commit/047de6860dd673cccd80076b395d762f9705f94f))
* add auth module sdk to validate autorization; :sparkles: ([c363fcc](https://github.com/LerianStudio/midaz/commit/c363fcc29587ab4292437ed2704017146ee58632))
* add flag feature to switch on/off telemetry; ([1b1cd10](https://github.com/LerianStudio/midaz/commit/1b1cd1008282913e71b013860329ac99e99886e1))
* add index to parent_transaction_id on transaction; ([83db867](https://github.com/LerianStudio/midaz/commit/83db867bd4384db2cf3a8cecfc05fb878ca644b4))
* add new implements to dockerfile and increase ttl on valkey; ([b1bfb12](https://github.com/LerianStudio/midaz/commit/b1bfb12e35a434f8b0568b6740ffcf0cffafe264))
* change redis image to valkey and some adjusts on docker-compose; ([0d6494c](https://github.com/LerianStudio/midaz/commit/0d6494cd580693d321e164e1537425e2ea43ee61))
* create new index from database; remove dockerfile platform to predefined redundant; ([8791e04](https://github.com/LerianStudio/midaz/commit/8791e042ef82aa03fa84b2015cf2077ac0685a25))
* enhance release notifications for beta and stable releases on discord ([394947c](https://github.com/LerianStudio/midaz/commit/394947c4e80600e877a43bed06bca1a6d6ed1c7e))
* Implements feature to get balance on database or redis on cache ([b507a9d](https://github.com/LerianStudio/midaz/commit/b507a9da54f1972936765f9729c330b5c8d2c9ff))
* test create image when open pr; ([d90aaf5](https://github.com/LerianStudio/midaz/commit/d90aaf568de1fe2fbfaae6a185c8ba09e6aa9672))
* use the account id if the alias is nil; ([5c8e8f3](https://github.com/LerianStudio/midaz/commit/5c8e8f3bca4012ef66dce5347f09ccd817707874))


### Bug Fixes

* add multi-arch to docker hub; ([69640e6](https://github.com/LerianStudio/midaz/commit/69640e6c87fbba0edd3e93dfbb10863f4fc4964e))
* add validation nil; :bug: ([b4aaa04](https://github.com/LerianStudio/midaz/commit/b4aaa042a8acb64eef2464153853106fd664017c))
* adjust erros struct fields to lower case names; :bug: ([b5a9620](https://github.com/LerianStudio/midaz/commit/b5a9620e80c40d336d67a6125ee5f87fc50ef2b0))
* cannot permit use same organization id as parent; :bug: ([03b160c](https://github.com/LerianStudio/midaz/commit/03b160ca657661060829e20529ada3123e821d28))
* change lib auth-sdk to auth-lib and replace on code; :bug: ([8644a5c](https://github.com/LerianStudio/midaz/commit/8644a5ca547f54fb8cdaa03fb6022e45256bf0fc))
* comment integrations test to refactor mdz after implement auth; ([d19c527](https://github.com/LerianStudio/midaz/commit/d19c5277df2db47603a5b900d31c190ca1cef374))
* create index to transaction_id to operation; ([b94ba29](https://github.com/LerianStudio/midaz/commit/b94ba295b47ae76b34940e54fb15330ba518ef27))
* discord mentions only when surrounded doble pipes ([70322e6](https://github.com/LerianStudio/midaz/commit/70322e6ddec05eecdc42073c24df5adc9c69a8f8))
* go sec; ([f18f629](https://github.com/LerianStudio/midaz/commit/f18f62931a9bbb340378601932865fe5144bbd46))
* make lint; ([b454376](https://github.com/LerianStudio/midaz/commit/b4543764ee6c3b4e7908fd455a01f553a2716b08))
* makefiles and standardize the commands across all of them ([#566](https://github.com/LerianStudio/midaz/issues/566)) ([4d02cbe](https://github.com/LerianStudio/midaz/commit/4d02cbec22b325ebfac6a4958a5d3ee93bc7888e))
* mongo max pool conn adjust and postgres add on config; ([ec09c48](https://github.com/LerianStudio/midaz/commit/ec09c482ef10c78f9c8b2df2d26f88ab5d313e43))
* remove a inexistent lint usetesting to tenv; :bug: ([68bf4d5](https://github.com/LerianStudio/midaz/commit/68bf4d52fb6c9498e49567417726deb9d2215732))
* remove auth from template deploy; ([e692244](https://github.com/LerianStudio/midaz/commit/e692244d1a818d50b61e0279fb6e90595d344989))
* remove auth url from use on sdk; :bug: ([aa2a7e2](https://github.com/LerianStudio/midaz/commit/aa2a7e268c7ec031af623bfd759789db32b2aead))
* remove trillian; ([8141a70](https://github.com/LerianStudio/midaz/commit/8141a703d4d9ade594be1f24108cd58de713bb2a))
* revert to when generating tags; ([daef69f](https://github.com/LerianStudio/midaz/commit/daef69fbfac87c7a6c179778edd90838f9ceb2eb))
* update auth-lib to newest version; :bug: ([9827ee4](https://github.com/LerianStudio/midaz/commit/9827ee4a3bd5852912141f3f5e3953f70851bccd))
* update go mod and go sum; ([681589e](https://github.com/LerianStudio/midaz/commit/681589e40e1fa4de0fd64a6f4d88d1e718f74e67))
* update go mod and sum; :bug: ([0ba596d](https://github.com/LerianStudio/midaz/commit/0ba596dd11cc99fb210d34af0231728412c8b629))
* update lint; :bug: ([58ced6f](https://github.com/LerianStudio/midaz/commit/58ced6fefc43be9deb984e2a9c6e0c2a2f1af49e))

## [1.49.0-beta.13](https://github.com/LerianStudio/midaz/compare/v1.49.0-beta.12...v1.49.0-beta.13) (2025-03-07)


### Bug Fixes

* discord mentions only when surrounded doble pipes ([70322e6](https://github.com/LerianStudio/midaz/commit/70322e6ddec05eecdc42073c24df5adc9c69a8f8))

## [1.49.0-beta.12](https://github.com/LerianStudio/midaz/compare/v1.49.0-beta.11...v1.49.0-beta.12) (2025-03-07)


### Features

* enhance release notifications for beta and stable releases on discord ([394947c](https://github.com/LerianStudio/midaz/commit/394947c4e80600e877a43bed06bca1a6d6ed1c7e))

## [1.49.0-beta.11](https://github.com/LerianStudio/midaz/compare/v1.49.0-beta.10...v1.49.0-beta.11) (2025-03-07)


### Features

* use the account id if the alias is nil; ([5c8e8f3](https://github.com/LerianStudio/midaz/commit/5c8e8f3bca4012ef66dce5347f09ccd817707874))


### Bug Fixes

* add validation nil; :bug: ([b4aaa04](https://github.com/LerianStudio/midaz/commit/b4aaa042a8acb64eef2464153853106fd664017c))
* cannot permit use same organization id as parent; :bug: ([03b160c](https://github.com/LerianStudio/midaz/commit/03b160ca657661060829e20529ada3123e821d28))
* update auth-lib to newest version; :bug: ([9827ee4](https://github.com/LerianStudio/midaz/commit/9827ee4a3bd5852912141f3f5e3953f70851bccd))
* update lint; :bug: ([58ced6f](https://github.com/LerianStudio/midaz/commit/58ced6fefc43be9deb984e2a9c6e0c2a2f1af49e))

## [1.49.0-beta.10](https://github.com/LerianStudio/midaz/compare/v1.49.0-beta.9...v1.49.0-beta.10) (2025-03-06)


### Features

* add a validations to cannot delete balance with funds; :sparkles: ([047de68](https://github.com/LerianStudio/midaz/commit/047de6860dd673cccd80076b395d762f9705f94f))


### Bug Fixes

* update go mod and sum; :bug: ([0ba596d](https://github.com/LerianStudio/midaz/commit/0ba596dd11cc99fb210d34af0231728412c8b629))

## [1.49.0-beta.9](https://github.com/LerianStudio/midaz/compare/v1.49.0-beta.8...v1.49.0-beta.9) (2025-03-06)


### Bug Fixes

* adjust erros struct fields to lower case names; :bug: ([b5a9620](https://github.com/LerianStudio/midaz/commit/b5a9620e80c40d336d67a6125ee5f87fc50ef2b0))
* change lib auth-sdk to auth-lib and replace on code; :bug: ([8644a5c](https://github.com/LerianStudio/midaz/commit/8644a5ca547f54fb8cdaa03fb6022e45256bf0fc))
* makefiles and standardize the commands across all of them ([#566](https://github.com/LerianStudio/midaz/issues/566)) ([4d02cbe](https://github.com/LerianStudio/midaz/commit/4d02cbec22b325ebfac6a4958a5d3ee93bc7888e))
* remove a inexistent lint usetesting to tenv; :bug: ([68bf4d5](https://github.com/LerianStudio/midaz/commit/68bf4d52fb6c9498e49567417726deb9d2215732))

## [1.49.0-beta.8](https://github.com/LerianStudio/midaz/compare/v1.49.0-beta.7...v1.49.0-beta.8) (2025-03-05)


### Features

* add auth module sdk to validate autorization; :sparkles: ([c363fcc](https://github.com/LerianStudio/midaz/commit/c363fcc29587ab4292437ed2704017146ee58632))
* add index to parent_transaction_id on transaction; ([83db867](https://github.com/LerianStudio/midaz/commit/83db867bd4384db2cf3a8cecfc05fb878ca644b4))


### Bug Fixes

* create index to transaction_id to operation; ([b94ba29](https://github.com/LerianStudio/midaz/commit/b94ba295b47ae76b34940e54fb15330ba518ef27))
* remove auth url from use on sdk; :bug: ([aa2a7e2](https://github.com/LerianStudio/midaz/commit/aa2a7e268c7ec031af623bfd759789db32b2aead))

## [1.49.0-beta.7](https://github.com/LerianStudio/midaz/compare/v1.49.0-beta.6...v1.49.0-beta.7) (2025-02-28)


### Features

* add flag feature to switch on/off telemetry; ([1b1cd10](https://github.com/LerianStudio/midaz/commit/1b1cd1008282913e71b013860329ac99e99886e1))
* create new index from database; remove dockerfile platform to predefined redundant; ([8791e04](https://github.com/LerianStudio/midaz/commit/8791e042ef82aa03fa84b2015cf2077ac0685a25))


### Bug Fixes

* go sec; ([f18f629](https://github.com/LerianStudio/midaz/commit/f18f62931a9bbb340378601932865fe5144bbd46))
* mongo max pool conn adjust and postgres add on config; ([ec09c48](https://github.com/LerianStudio/midaz/commit/ec09c482ef10c78f9c8b2df2d26f88ab5d313e43))
* remove trillian; ([8141a70](https://github.com/LerianStudio/midaz/commit/8141a703d4d9ade594be1f24108cd58de713bb2a))

## [1.49.0-beta.6](https://github.com/LerianStudio/midaz/compare/v1.49.0-beta.5...v1.49.0-beta.6) (2025-02-27)


### Features

* add 5 works to consumer in rabbitmq and implement message persistent; ([1625d3b](https://github.com/LerianStudio/midaz/commit/1625d3b4aa22ad0889abdd23c97fa41e17714668))


### Bug Fixes

* make lint; ([b454376](https://github.com/LerianStudio/midaz/commit/b4543764ee6c3b4e7908fd455a01f553a2716b08))

## [1.49.0-beta.5](https://github.com/LerianStudio/midaz/compare/v1.49.0-beta.4...v1.49.0-beta.5) (2025-02-27)


### Features

* add new implements to dockerfile and increase ttl on valkey; ([b1bfb12](https://github.com/LerianStudio/midaz/commit/b1bfb12e35a434f8b0568b6740ffcf0cffafe264))

## [1.49.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.49.0-beta.3...v1.49.0-beta.4) (2025-02-27)


### Features

* change redis image to valkey and some adjusts on docker-compose; ([0d6494c](https://github.com/LerianStudio/midaz/commit/0d6494cd580693d321e164e1537425e2ea43ee61))
* test create image when open pr; ([d90aaf5](https://github.com/LerianStudio/midaz/commit/d90aaf568de1fe2fbfaae6a185c8ba09e6aa9672))


### Bug Fixes

* add multi-arch to docker hub; ([69640e6](https://github.com/LerianStudio/midaz/commit/69640e6c87fbba0edd3e93dfbb10863f4fc4964e))
* revert to when generating tags; ([daef69f](https://github.com/LerianStudio/midaz/commit/daef69fbfac87c7a6c179778edd90838f9ceb2eb))

## [1.49.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.49.0-beta.2...v1.49.0-beta.3) (2025-02-26)


### Features

* Implements feature to get balance on database or redis on cache ([b507a9d](https://github.com/LerianStudio/midaz/commit/b507a9da54f1972936765f9729c330b5c8d2c9ff))


### Bug Fixes

* update go mod and go sum; ([681589e](https://github.com/LerianStudio/midaz/commit/681589e40e1fa4de0fd64a6f4d88d1e718f74e67))

## [1.49.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.49.0-beta.1...v1.49.0-beta.2) (2025-02-25)


### Bug Fixes

* comment integrations test to refactor mdz after implement auth; ([d19c527](https://github.com/LerianStudio/midaz/commit/d19c5277df2db47603a5b900d31c190ca1cef374))
* remove auth from template deploy; ([e692244](https://github.com/LerianStudio/midaz/commit/e692244d1a818d50b61e0279fb6e90595d344989))

## [1.49.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.48.0...v1.49.0-beta.1) (2025-02-24)

## [1.48.0](https://github.com/LerianStudio/midaz/compare/v1.47.0...v1.48.0) (2025-02-21)


### Features

* add acid to select for update; ([17b02aa](https://github.com/LerianStudio/midaz/commit/17b02aa091be7b17077bdeeffd538f791bad2de6))
* add balance docs :sparkles: ([a6c6560](https://github.com/LerianStudio/midaz/commit/a6c6560c13f914027b06f892fd9b17dda078f787))
* add balance functions :sparkles: ([92f35ea](https://github.com/LerianStudio/midaz/commit/92f35ea866a9c64f7c22dd17157e5916d4e000ee))
* add balance routes :sparkles: ([23da369](https://github.com/LerianStudio/midaz/commit/23da369836b23e13ebddd14268fe8ab9c43d8a53))
* add balance, transaction and operations to be persisted in a queue; ([c55d348](https://github.com/LerianStudio/midaz/commit/c55d348aba14425d41480e99efb57be5b165608c))
* add devops to codeowner when change .env.example files; :sparkles: ([e1328f8](https://github.com/LerianStudio/midaz/commit/e1328f8117f68ecebad7f5fe65a843ace78d30ab))
* add new table balance for perfomance poc; ([cd1bc6b](https://github.com/LerianStudio/midaz/commit/cd1bc6b846615b1d8997dadc6e1887c238e82e6b))
* add on asset and account when creation account to send to the transaction balance queue; :sparkles: ([aeedcf9](https://github.com/LerianStudio/midaz/commit/aeedcf9ef366bd9e7e8f11ecb9b675c2cfc3438f))
* add optimistic lock using version on database; :sparkles: ([bd753c9](https://github.com/LerianStudio/midaz/commit/bd753c9e87023a6db100f31e050247cb29b6b1ff))
* add rabbit queue; create producer and consumer to retry; ([2c72336](https://github.com/LerianStudio/midaz/commit/2c723367d26000f38a8adc71d615eac80f1b991e))
* add tests :sparkles: :sparkles: ([c032548](https://github.com/LerianStudio/midaz/commit/c032548a8fb22e1fbefcdddc2ea791790fe50d32))
* add trace on casdoor; :sparkles: ([63d084d](https://github.com/LerianStudio/midaz/commit/63d084d473484e7585e4c22c48675b8100c2de53))
* certificate the env version ([7415f49](https://github.com/LerianStudio/midaz/commit/7415f49e01387e0b4f495283d1ba585cd16e822a))
* change structure ([6036bb8](https://github.com/LerianStudio/midaz/commit/6036bb8eef6b1828fe72f45ea92037b47fb73f91))
* change structure of file ([488eebd](https://github.com/LerianStudio/midaz/commit/488eebd5fb7537d41823e28860a9e2056a29928f))
* change the env ([120f768](https://github.com/LerianStudio/midaz/commit/120f7687ee10343396d52e857842eadc6c5241e9))
* change the ENV VARS to test the flow ([7fb001b](https://github.com/LerianStudio/midaz/commit/7fb001bbfbe38da9fe7a5c1a98e0958e0a8a0bc2))
* change the flow ([ff2a573](https://github.com/LerianStudio/midaz/commit/ff2a573546ec453d241a867347d340cf8e241a98))
* change the flow to track all the folders on components ([c0e343d](https://github.com/LerianStudio/midaz/commit/c0e343d5184882e9a6125507ba0b655993bba910))
* change the LOG_LEVEL NEV to test ([8205ef8](https://github.com/LerianStudio/midaz/commit/8205ef8b6363eb63908b4ffb7bef99aeddbab1d6))
* change the structure of file ([b6441d0](https://github.com/LerianStudio/midaz/commit/b6441d03504a477d54eb71d161c22d7f3a994c48))
* change the trigger to pull request to branch main ([6265a7d](https://github.com/LerianStudio/midaz/commit/6265a7d5011e44c6e45d262d693c92d978b97f50))
* change the value of VAR to test ([727248c](https://github.com/LerianStudio/midaz/commit/727248c327db4fd413adba6564fc5a5b4d304c4d))
* change the VAR version to test ([5cd3cc2](https://github.com/LerianStudio/midaz/commit/5cd3cc24dc6a397e9c536d4b2ade32fc38e74393))
* change the version ([f07ff76](https://github.com/LerianStudio/midaz/commit/f07ff76799a9d19997bbfd5641d138cd020d1d47))
* change the while to validate for ([1716a8b](https://github.com/LerianStudio/midaz/commit/1716a8bca178c290fbec137c80b1cc2532a246af))
* changing the VERSION VAR ([8a7d7e9](https://github.com/LerianStudio/midaz/commit/8a7d7e99eff4db2d28996919a9139734c68219fb))
* check changing the env version ([22d45a7](https://github.com/LerianStudio/midaz/commit/22d45a73462ed5f490c1b38917ce473eb9411721))
* check difference between files ([5fc7bdc](https://github.com/LerianStudio/midaz/commit/5fc7bdcec0e73b442e4c90f36df8df8441bc2780))
* check the history of commits ([b588038](https://github.com/LerianStudio/midaz/commit/b58803873c435584f15523ecd620076287f03f9a))
* check version ([98f6c9e](https://github.com/LerianStudio/midaz/commit/98f6c9e608b8e79168c95cdb8075cf08c89d7264))
* checking the version field ([7af2a00](https://github.com/LerianStudio/midaz/commit/7af2a00313dddfd09929ce7c388d53f8528e0f39))
* clarify the ids of steps on the flow ([b9c6df9](https://github.com/LerianStudio/midaz/commit/b9c6df96449b80ad550483fbe1efd199f6c186fc))
* close the while for ([ac183db](https://github.com/LerianStudio/midaz/commit/ac183dba75b0c4b3905440b99e8fa84e51303b16))
* create balance sql implements; ([e89d870](https://github.com/LerianStudio/midaz/commit/e89d870a7c4c6b42a77fd0331889cbd061dc43db))
* create structure to receive account from rabbitmq and create balance; :sparkles: ([6a3b41c](https://github.com/LerianStudio/midaz/commit/6a3b41c847f8de3e25f146040e8c9d43d0d3390f))
* exclude the version VAR ([acfd652](https://github.com/LerianStudio/midaz/commit/acfd6528ca34cbf71ce234b67e873a0bf2d25fc4))
* exclude the VERSION var ([391516c](https://github.com/LerianStudio/midaz/commit/391516c0890f8caed580ef7f81059c5dd437a412))
* execute the test to notification ([f26f3e0](https://github.com/LerianStudio/midaz/commit/f26f3e0ee80714178fc9f405bb9cbd0b26296c8b))
* first version of select of update with balance rules; :sparkles: ([0bcde2e](https://github.com/LerianStudio/midaz/commit/0bcde2ea41e10aec0697dfe214dbd6383ce06824))
* force to execute env var prs ([9dda835](https://github.com/LerianStudio/midaz/commit/9dda835dc1f4d1fcf0b2fc57c708649cceee3ae8))
* insert flow notification on ENV VARS changes ([6388b47](https://github.com/LerianStudio/midaz/commit/6388b47b2a2972dc3522dcfc14d29ce553ab840b))
* insert git fetch prune on flow ([645f0f9](https://github.com/LerianStudio/midaz/commit/645f0f970c8c4557922935bb24c039ea5f505110))
* maintain ledger folder ([d386e0b](https://github.com/LerianStudio/midaz/commit/d386e0b779f2d1aaf73803f200c8bf71a603bafd))
* mantain the correct version on audit .env.example ([91921f8](https://github.com/LerianStudio/midaz/commit/91921f875d72ca51711d740bd77470a022cec5af))
* pgbouncer and 3000 conn and shared buffers 1gb; :sparkles: ([6eceeef](https://github.com/LerianStudio/midaz/commit/6eceeefb163ca5252de4ced9c415e865addb5c63))
* pgbouncer try to config connections to best tps possible; :sparkles: ([fa59836](https://github.com/LerianStudio/midaz/commit/fa59836f81af8ec4acb520dac172c1bafd404cb3))
* return the values to default ([87187f0](https://github.com/LerianStudio/midaz/commit/87187f09de47d3fc67fcb2c604d4067cff545f00))
* select for update with version correctly implemented; :sparkles: ([a7e10ff](https://github.com/LerianStudio/midaz/commit/a7e10ff58ed0b12a77e1f2126d30836e7703dc6d))
* set the changes on file ([91200b9](https://github.com/LerianStudio/midaz/commit/91200b9e778e985cd5c53bb19acdb388d5bc4b9e))
* set the changes on file ([11a5b75](https://github.com/LerianStudio/midaz/commit/11a5b755d81842d47b5e9e41ba80ab8bb0663793))
* set the changes on file ([964d5d5](https://github.com/LerianStudio/midaz/commit/964d5d53f9c5f4169ef814e494ff31ae52589fb4))
* set the command to compare and verify ([fdcfab0](https://github.com/LerianStudio/midaz/commit/fdcfab0165fee5bdf218abb52df9d9c925462a9b))
* set the compare ([e432447](https://github.com/LerianStudio/midaz/commit/e4324476eab36832b1677cd3ab2539f195542275))
* set the comparison of commits ([e5f9c1a](https://github.com/LerianStudio/midaz/commit/e5f9c1abeb0b34422e7ba054346523d0669fa45a))
* set the configuration ([357bfa6](https://github.com/LerianStudio/midaz/commit/357bfa6993b624794ee1bbd459ca7afbdaa549e4))
* set the configuration and tests ([b3d1361](https://github.com/LerianStudio/midaz/commit/b3d1361413bc07ce277bf9293dabb4992a42fea6))
* set the configuration of message on slack ([197aad1](https://github.com/LerianStudio/midaz/commit/197aad1a2a486488eb5c9f9aa91d2d4fda2fee40))
* set the current version ([3cda634](https://github.com/LerianStudio/midaz/commit/3cda63454ee7b1cd82da62a0226f0d8ab5fd45aa))
* set the diff of versions ([6cf4205](https://github.com/LerianStudio/midaz/commit/6cf4205d976842293edc7e1ee0e4adf6436a3ad0))
* set the env vars ([2b2da89](https://github.com/LerianStudio/midaz/commit/2b2da8970e82338f78aa6b8effca3219ac42b6f1))
* set the estructure verification ([ac5a974](https://github.com/LerianStudio/midaz/commit/ac5a974e4dc77d2dfa073abbd2b90413fa923afe))
* set the files to compare ([d63d071](https://github.com/LerianStudio/midaz/commit/d63d0713d770f724c9da8827e96632a6f53f0f64))
* set the flow of changes ([edd5561](https://github.com/LerianStudio/midaz/commit/edd55612e01451b4734c5fa55df2c79b404467ca))
* set the flow to test file ([65e551f](https://github.com/LerianStudio/midaz/commit/65e551f9d013cc23d27618bee84977eafad718e4))
* set the flow verifying branch ([592d3ad](https://github.com/LerianStudio/midaz/commit/592d3ad981c6d305720742137096e6503a5762fe))
* set the identation ([3776939](https://github.com/LerianStudio/midaz/commit/37769390cf56289d45fff2a16cf9ba87c13fb327))
* set the regex ([fccf47f](https://github.com/LerianStudio/midaz/commit/fccf47f7c037b49b2bc3add4a70546ba950e4daf))
* set the structure ([bf0c621](https://github.com/LerianStudio/midaz/commit/bf0c621d1f864f32058ba4fd3361b769e4aa7ca7))
* set the structure of envs ([838164c](https://github.com/LerianStudio/midaz/commit/838164c196d8da7fa7068a6ad78228f9c1b0d0a9))
* set the structure of github action workflow ([8823bf1](https://github.com/LerianStudio/midaz/commit/8823bf1764afff4759b6e836cd18842e3cefc98e))
* set the structure of github actions ([5fbff7d](https://github.com/LerianStudio/midaz/commit/5fbff7def3e556bce04f379a60456a7ffadbe4a4))
* set the value of log level ([8302bae](https://github.com/LerianStudio/midaz/commit/8302bae876863cf066face808213217cf2abc6cd))
* set the var of audit to test ([e484c49](https://github.com/LerianStudio/midaz/commit/e484c49d403bb7709ff27d457e6389957867aaa0))
* set the verification exclude version ([2a18364](https://github.com/LerianStudio/midaz/commit/2a183647ac5824b55976f1ac32cb4a7078eda6c5))
* set the verification on VERSION var ([e1eaf0c](https://github.com/LerianStudio/midaz/commit/e1eaf0cc01358466b9e2dd7b5848bf93978acddb))
* set the version ([009d101](https://github.com/LerianStudio/midaz/commit/009d1010860a79cc9d705e36262543436ebe3aef))
* set the version ([1881dc0](https://github.com/LerianStudio/midaz/commit/1881dc0b1f8388ed45a3676d9a12d4e47f70368c))
* set the version ([e673903](https://github.com/LerianStudio/midaz/commit/e6739034d71bc49d8eff4c5e428407762eac1a9e))
* set the version ([d07d7d5](https://github.com/LerianStudio/midaz/commit/d07d7d57b2ddd89a71e579001ecd089f0b7bd75a))
* set the version of file ([71ed8f7](https://github.com/LerianStudio/midaz/commit/71ed8f7fc68ac6cbbc55fcadb91d8822597b6fc5))
* set the version to test ([22f7dab](https://github.com/LerianStudio/midaz/commit/22f7dab90d94f62ce9e206edb96c663d92ae1286))
* set the versions ([e4f1c0e](https://github.com/LerianStudio/midaz/commit/e4f1c0efa02a1c14f720f6c05c95d77c2d4b813a))
* set version of file ([a5247e3](https://github.com/LerianStudio/midaz/commit/a5247e3762ad204e0f1e4fac469dcc040f9e4e9d))
* simulate the app bot to increase VERSION var ([e9f97aa](https://github.com/LerianStudio/midaz/commit/e9f97aa06222f234bdc202a3bd432cfdf0861269))
* test changing the log level VAR ([4020a32](https://github.com/LerianStudio/midaz/commit/4020a329626b0121cad4471cf535935ae05ef813))
* test the change on env. example files ([97d2c95](https://github.com/LerianStudio/midaz/commit/97d2c950a35ec7c87e0fc878f21820f2dd0107ab))
* test the execution flow notificate env vars changes ([a1b6b07](https://github.com/LerianStudio/midaz/commit/a1b6b07ce771e5ba0c00d7b39e0d2207667433a2))
* test the structure ([2701552](https://github.com/LerianStudio/midaz/commit/27015523966dbb37c5289b1166bdc871d27dc9d1))
* test the VERSION variable ([7e81a5a](https://github.com/LerianStudio/midaz/commit/7e81a5ad18f61e5c7defbeb7ef08ac03cf11fbdd))
* test the workflow changing ENV VARS on scenario test ([a3e8552](https://github.com/LerianStudio/midaz/commit/a3e855240a04e680812e8a9cd9d8e5dac129b6b3))
* the the flow to notify devops team ([847b4b2](https://github.com/LerianStudio/midaz/commit/847b4b22b926304ab57f130f447d09ff1285db47))
* update balance async; rename validate accounts to validate balance rules; :sparkles: ([587f362](https://github.com/LerianStudio/midaz/commit/587f3622bcfdc8f23432cec3d219abc6c5fdc5ce))
* update postman :sparkles: ([070f01f](https://github.com/LerianStudio/midaz/commit/070f01f1426f84581521d221e5b2e52d2595a5f8))
* using identation ([9e88384](https://github.com/LerianStudio/midaz/commit/9e88384bc1e224df361bf02b18b0f3ccb013d85e))
* verify current branch ([59ef127](https://github.com/LerianStudio/midaz/commit/59ef1278fe96b193586dd12e100e40670325d077))
* verify env. example changes ([82a0926](https://github.com/LerianStudio/midaz/commit/82a09261f00e734813d38c750887809f739dd798))
* verify the change of the flow ([c7ca86e](https://github.com/LerianStudio/midaz/commit/c7ca86ed2dd9e8264e23de608d719c0fc4a28530))
* verify the changes ([d85473b](https://github.com/LerianStudio/midaz/commit/d85473bd324bc5d40f8ae55de80c3f67b9b9127d))
* verify version ([f22eec0](https://github.com/LerianStudio/midaz/commit/f22eec0787cf1b5f3c8ba32ef9bfd98448a55407))
* verify version ([3eabb84](https://github.com/LerianStudio/midaz/commit/3eabb84fe5e252b21ba4155bf5fb0f45be22395b))
* verify VERSION ([c9e44dc](https://github.com/LerianStudio/midaz/commit/c9e44dce8d9bf3b93594f698c863b432b43d5b07))


### Bug Fixes

* add balance_id to knows paths parameters; :bug: ([f999782](https://github.com/LerianStudio/midaz/commit/f999782767a715ee29523a1823a4b901be7f89c0))
* add column that accepts account to be negative; :bug: ([e952a37](https://github.com/LerianStudio/midaz/commit/e952a379a3236c56dc739be72e5e8909e5b3a1a6))
* add dev too to validate pr in .envs :bug: ([0aae153](https://github.com/LerianStudio/midaz/commit/0aae153ca9ce3665fea4bd3317111a15d58f595b))
* add insert values on table; :bug: ([e23a9ab](https://github.com/LerianStudio/midaz/commit/e23a9abefe9ccdbe6d866e69beb2fc250b606f42))
* add log to catch erros when cannot marshal result var; :bug: ([1074626](https://github.com/LerianStudio/midaz/commit/10746263fffb4296cb08784b8d617196397e5130))
* add omitempty that when body pass metadata key with null remove from db; :bug: ([ed405c7](https://github.com/LerianStudio/midaz/commit/ed405c78d5ed489d73e0ec92e56a0e0088f71599))
* add pgbounce ([9cddd3e](https://github.com/LerianStudio/midaz/commit/9cddd3e5df1dafef06bb9b0494ed1d3dfe229eeb))
* add rule to only lock balances on redis if has more than one balance on slice; :bug: ([1c52e66](https://github.com/LerianStudio/midaz/commit/1c52e66c12e44d5c2451ca1c8fc484cb6054b213))
* add two index one by alias e another by account_id; :bug: ([f6b36ab](https://github.com/LerianStudio/midaz/commit/f6b36ab32c777156d94aed68add262592fd05bc2))
* add validation to avoid ambiguous account on source and distribute; :bug: ([d201dec](https://github.com/LerianStudio/midaz/commit/d201dec63c9c6fbf91a8b03dac0aea45a247ccd8))
* adjust accounts removing old fields and reorganizing it; :bug: ([b86d541](https://github.com/LerianStudio/midaz/commit/b86d5411309ecb7116ea3305a048cc3592021b42))
* adjust balance update to only update a individual field; :bug: ([c460707](https://github.com/LerianStudio/midaz/commit/c460707fafc8b746aaf7fe4414840a50c08fd6f5))
* adjust go test after change asset and account; :bug: :bug: ([6bddbea](https://github.com/LerianStudio/midaz/commit/6bddbea6ac442f9a76276674c64f881ce7bad7ca))
* adjust legal_name and doing_business_as to be updated individually; :bug: ([bea1039](https://github.com/LerianStudio/midaz/commit/bea1039274d30154be73993a07a2b40c5378ecec))
* adjust lint; :bug: ([b946a43](https://github.com/LerianStudio/midaz/commit/b946a43472c3e9e2ccf5141a27598a0bb4635d78))
* audit log enabled false; :bug: ([f7ca26b](https://github.com/LerianStudio/midaz/commit/f7ca26b78126e9f1410cb079e37f1395ac5cb4c7))
* because body field is json:"-" in transaction when do marshal field desapear; :bug: ([9d4c563](https://github.com/LerianStudio/midaz/commit/9d4c563059d420baa1f327edfb12b024745f87dd))
* change fatal to error; :bug: ([326f235](https://github.com/LerianStudio/midaz/commit/326f235b124914fa0cda9927216fc06488ad9903))
* change func to log erro ands pass make lint; :bug: ([bfdbdb9](https://github.com/LerianStudio/midaz/commit/bfdbdb979d482e81fc57221d2681c1175f756c3c))
* change reference name ledger to onboarding; :bug: :bug: ([2568d2b](https://github.com/LerianStudio/midaz/commit/2568d2b313b8cabfefef36bebc74473117b5827d))
* make lint and make sec; :bug: ([5d83b0d](https://github.com/LerianStudio/midaz/commit/5d83b0dff59175d323b9da77681efa6a3ae453fb))
* make lint and make sec; refactor swagger and openapi; :bug: ([5b07249](https://github.com/LerianStudio/midaz/commit/5b07249ebcb1a8142aeed50f4b6b301e0fc8081c))
* make name required when creating asset :bug: ([58f7fd6](https://github.com/LerianStudio/midaz/commit/58f7fd6bd39359ad26e56b3392c3a5a6c145bcc9))
* make sec and make lint; :bug: ([3eacf02](https://github.com/LerianStudio/midaz/commit/3eacf0274b25cedfb1714d771dbbd3e98fa7ffcb))
* make test; :bug: ([44b8a38](https://github.com/LerianStudio/midaz/commit/44b8a38d7e784d37556916d3da5da9486343310e))
* message casdoor name; :bug: ([a1e9431](https://github.com/LerianStudio/midaz/commit/a1e943183ea4db6bdde33a3a39e3d5e2dc1dd681))
* midazId name; :bug: ([263cae3](https://github.com/LerianStudio/midaz/commit/263cae37cb5b5f308f1aaff64255349b8e1d29af))
* param sort order :bug: ([610167a](https://github.com/LerianStudio/midaz/commit/610167a03d584313a85e277a7bd28dd9a3c00626))
* pgbounce shared buffers on replica; :bug: ([fe91059](https://github.com/LerianStudio/midaz/commit/fe910596423740ce7fc91cb7440a00397ef6952b))
* put bto sync; :bug: ([e028f41](https://github.com/LerianStudio/midaz/commit/e028f413f432a6281da2d09a70d90a92da79e17c))
* remove extension; add index; :bug: ([8d06d22](https://github.com/LerianStudio/midaz/commit/8d06d22f70120932632fdb537f8b05977f71d564))
* remove grpc accounts from ledger; :bug: ([7e91f64](https://github.com/LerianStudio/midaz/commit/7e91f64fa0d6c0442aac2dab8ad869706fb1c500))
* remove old locks rules; :bug: ([2bba68d](https://github.com/LerianStudio/midaz/commit/2bba68d8915b3d78bba77d1735c06fe53f172821))
* remove old references from accounts; :bug: ([dfb0a57](https://github.com/LerianStudio/midaz/commit/dfb0a57d1ed2fabab1e1c71f36c743fc5b61fa89))
* remove portfolio routes and funcs on account that was deprecated and update postman; :bug: ([7bc5f5e](https://github.com/LerianStudio/midaz/commit/7bc5f5e7f46465ef4d7fd415a685b3d9e680593d))
* remove protobuf door on ledger; :bug: ([b3d0b56](https://github.com/LerianStudio/midaz/commit/b3d0b566b2f039a362c51fac351d2c8f843c1efa))
* remove protobuf reference on ledger; :bug: ([1ccefc5](https://github.com/LerianStudio/midaz/commit/1ccefc5f1b15287a8a5c3711f5f73f99e5737382))
* reusable protobuf door on transaction and transaction door on audit; :bug: ([2224904](https://github.com/LerianStudio/midaz/commit/2224904540995a9bb2096dda7c904f4a7b3d4a2c))
* revert telemetry; :bug: ([2d1fbce](https://github.com/LerianStudio/midaz/commit/2d1fbcef56af103077171a8232eda9d48f612849))
* some adjusts to improve tps performance; ([57eb1c8](https://github.com/LerianStudio/midaz/commit/57eb1c83f23acb5eae23151470bbdb18e38e097b))
* tables accounts and balance; :bug: ([23f0e16](https://github.com/LerianStudio/midaz/commit/23f0e16f63e44e5c07aa6947c2ccdd2e7951f20b))
* update asset to remove old fields on create external account; :bug: ([ef34a89](https://github.com/LerianStudio/midaz/commit/ef34a89818da10f7fe63f5f004f903b402afa4f3))
* update balance create too to avoid erros and return msg to queue; :bug: ([cc0b38b](https://github.com/LerianStudio/midaz/commit/cc0b38be8e9b4028f40d7a13cf1dd4984b26c07d))
* update create transaction and operation erro handling duplicated key; :bug: ([2a984df](https://github.com/LerianStudio/midaz/commit/2a984dfa84d51607eab9cf5bab969dc3d212a695))
* update description log oeprations to operations; :bug: ([5fe3156](https://github.com/LerianStudio/midaz/commit/5fe3156b5eb6686ed9bc661fac5e123dee8b5d53))
* update dockerfile port; :bug: ([e3c17a2](https://github.com/LerianStudio/midaz/commit/e3c17a2a443aa38856ab70a0e599db7d923de48c))
* update golang dependencies based on dependabot; :bug: ([84af1ce](https://github.com/LerianStudio/midaz/commit/84af1ce6da5ccc9a54e78d09ff7b41388d4684a3))
* update libs on go mod and go sum; :bug: ([042951e](https://github.com/LerianStudio/midaz/commit/042951ea6c86f6eb91adef676ccee7fdb7356fc3))
* update ports docker; :bug: ([41260c9](https://github.com/LerianStudio/midaz/commit/41260c9d991a6e2b299f79409c5d9fb132596ff2))
* update rabbitmq queues names, exchanges and keys; :bug: ([e1f8c56](https://github.com/LerianStudio/midaz/commit/e1f8c56e66e5182ac51fadfa99d1b9796c0e67bd))
* update some changes to adjuste version on database; :bug: ([6f61294](https://github.com/LerianStudio/midaz/commit/6f612947dd3167b489270b35ea16ea69d4d57001))
* update swagger and openapi; :bug: ([d034c4e](https://github.com/LerianStudio/midaz/commit/d034c4e36a4c9994fab4cb43a023d2244b5142b3))
* update tests; :bug: ([8d919fe](https://github.com/LerianStudio/midaz/commit/8d919fe75ab159abfaac047ec9e7dcac55b1524a))
* version using queue and remove database optimistic lock; :bug: ([cc32b57](https://github.com/LerianStudio/midaz/commit/cc32b5705cc70be807675d25874dddda22f1a0a7))

## [1.48.0-beta.12](https://github.com/LerianStudio/midaz/compare/v1.48.0-beta.11...v1.48.0-beta.12) (2025-02-21)


### Bug Fixes

* update balance create too to avoid erros and return msg to queue; :bug: ([cc0b38b](https://github.com/LerianStudio/midaz/commit/cc0b38be8e9b4028f40d7a13cf1dd4984b26c07d))
* update create transaction and operation erro handling duplicated key; :bug: ([2a984df](https://github.com/LerianStudio/midaz/commit/2a984dfa84d51607eab9cf5bab969dc3d212a695))

## [1.48.0-beta.11](https://github.com/LerianStudio/midaz/compare/v1.48.0-beta.10...v1.48.0-beta.11) (2025-02-20)


### Bug Fixes

* because body field is json:"-" in transaction when do marshal field desapear; :bug: ([9d4c563](https://github.com/LerianStudio/midaz/commit/9d4c563059d420baa1f327edfb12b024745f87dd))

## [1.48.0-beta.10](https://github.com/LerianStudio/midaz/compare/v1.48.0-beta.9...v1.48.0-beta.10) (2025-02-20)


### Features

* add acid to select for update; ([17b02aa](https://github.com/LerianStudio/midaz/commit/17b02aa091be7b17077bdeeffd538f791bad2de6))
* first version of select of update with balance rules; :sparkles: ([0bcde2e](https://github.com/LerianStudio/midaz/commit/0bcde2ea41e10aec0697dfe214dbd6383ce06824))
* select for update with version correctly implemented; :sparkles: ([a7e10ff](https://github.com/LerianStudio/midaz/commit/a7e10ff58ed0b12a77e1f2126d30836e7703dc6d))


### Bug Fixes

* make lint and make sec; :bug: ([5d83b0d](https://github.com/LerianStudio/midaz/commit/5d83b0dff59175d323b9da77681efa6a3ae453fb))

## [1.48.0-beta.9](https://github.com/LerianStudio/midaz/compare/v1.48.0-beta.8...v1.48.0-beta.9) (2025-02-20)


### Bug Fixes

* change fatal to error; :bug: ([326f235](https://github.com/LerianStudio/midaz/commit/326f235b124914fa0cda9927216fc06488ad9903))
* change func to log erro ands pass make lint; :bug: ([bfdbdb9](https://github.com/LerianStudio/midaz/commit/bfdbdb979d482e81fc57221d2681c1175f756c3c))
* put bto sync; :bug: ([e028f41](https://github.com/LerianStudio/midaz/commit/e028f413f432a6281da2d09a70d90a92da79e17c))
* update rabbitmq queues names, exchanges and keys; :bug: ([e1f8c56](https://github.com/LerianStudio/midaz/commit/e1f8c56e66e5182ac51fadfa99d1b9796c0e67bd))
* update some changes to adjuste version on database; :bug: ([6f61294](https://github.com/LerianStudio/midaz/commit/6f612947dd3167b489270b35ea16ea69d4d57001))
* version using queue and remove database optimistic lock; :bug: ([cc32b57](https://github.com/LerianStudio/midaz/commit/cc32b5705cc70be807675d25874dddda22f1a0a7))

## [1.48.0-beta.8](https://github.com/LerianStudio/midaz/compare/v1.48.0-beta.7...v1.48.0-beta.8) (2025-02-19)


### Features

* add optimistic lock using version on database; :sparkles: ([bd753c9](https://github.com/LerianStudio/midaz/commit/bd753c9e87023a6db100f31e050247cb29b6b1ff))

## [1.48.0-beta.7](https://github.com/LerianStudio/midaz/compare/v1.48.0-beta.6...v1.48.0-beta.7) (2025-02-18)


### Features

* add balance, transaction and operations to be persisted in a queue; ([c55d348](https://github.com/LerianStudio/midaz/commit/c55d348aba14425d41480e99efb57be5b165608c))
* add rabbit queue; create producer and consumer to retry; ([2c72336](https://github.com/LerianStudio/midaz/commit/2c723367d26000f38a8adc71d615eac80f1b991e))
* pgbouncer and 3000 conn and shared buffers 1gb; :sparkles: ([6eceeef](https://github.com/LerianStudio/midaz/commit/6eceeefb163ca5252de4ced9c415e865addb5c63))
* pgbouncer try to config connections to best tps possible; :sparkles: ([fa59836](https://github.com/LerianStudio/midaz/commit/fa59836f81af8ec4acb520dac172c1bafd404cb3))


### Bug Fixes

* add pgbounce ([9cddd3e](https://github.com/LerianStudio/midaz/commit/9cddd3e5df1dafef06bb9b0494ed1d3dfe229eeb))
* adjust lint; :bug: ([b946a43](https://github.com/LerianStudio/midaz/commit/b946a43472c3e9e2ccf5141a27598a0bb4635d78))
* audit log enabled false; :bug: ([f7ca26b](https://github.com/LerianStudio/midaz/commit/f7ca26b78126e9f1410cb079e37f1395ac5cb4c7))
* message casdoor name; :bug: ([a1e9431](https://github.com/LerianStudio/midaz/commit/a1e943183ea4db6bdde33a3a39e3d5e2dc1dd681))
* pgbounce shared buffers on replica; :bug: ([fe91059](https://github.com/LerianStudio/midaz/commit/fe910596423740ce7fc91cb7440a00397ef6952b))
* some adjusts to improve tps performance; ([57eb1c8](https://github.com/LerianStudio/midaz/commit/57eb1c83f23acb5eae23151470bbdb18e38e097b))

## [1.48.0-beta.6](https://github.com/LerianStudio/midaz/compare/v1.48.0-beta.5...v1.48.0-beta.6) (2025-02-18)


### Features

* add balance docs :sparkles: ([a6c6560](https://github.com/LerianStudio/midaz/commit/a6c6560c13f914027b06f892fd9b17dda078f787))
* add balance functions :sparkles: ([92f35ea](https://github.com/LerianStudio/midaz/commit/92f35ea866a9c64f7c22dd17157e5916d4e000ee))
* add balance routes :sparkles: ([23da369](https://github.com/LerianStudio/midaz/commit/23da369836b23e13ebddd14268fe8ab9c43d8a53))
* add tests :sparkles: :sparkles: ([c032548](https://github.com/LerianStudio/midaz/commit/c032548a8fb22e1fbefcdddc2ea791790fe50d32))
* update postman :sparkles: ([070f01f](https://github.com/LerianStudio/midaz/commit/070f01f1426f84581521d221e5b2e52d2595a5f8))


### Bug Fixes

* add balance_id to knows paths parameters; :bug: ([f999782](https://github.com/LerianStudio/midaz/commit/f999782767a715ee29523a1823a4b901be7f89c0))
* add omitempty that when body pass metadata key with null remove from db; :bug: ([ed405c7](https://github.com/LerianStudio/midaz/commit/ed405c78d5ed489d73e0ec92e56a0e0088f71599))
* adjust balance update to only update a individual field; :bug: ([c460707](https://github.com/LerianStudio/midaz/commit/c460707fafc8b746aaf7fe4414840a50c08fd6f5))
* adjust legal_name and doing_business_as to be updated individually; :bug: ([bea1039](https://github.com/LerianStudio/midaz/commit/bea1039274d30154be73993a07a2b40c5378ecec))
* make lint and make sec; refactor swagger and openapi; :bug: ([5b07249](https://github.com/LerianStudio/midaz/commit/5b07249ebcb1a8142aeed50f4b6b301e0fc8081c))
* make test; :bug: ([44b8a38](https://github.com/LerianStudio/midaz/commit/44b8a38d7e784d37556916d3da5da9486343310e))
* param sort order :bug: ([610167a](https://github.com/LerianStudio/midaz/commit/610167a03d584313a85e277a7bd28dd9a3c00626))
* update libs on go mod and go sum; :bug: ([042951e](https://github.com/LerianStudio/midaz/commit/042951ea6c86f6eb91adef676ccee7fdb7356fc3))
* update tests; :bug: ([8d919fe](https://github.com/LerianStudio/midaz/commit/8d919fe75ab159abfaac047ec9e7dcac55b1524a))

## [1.48.0-beta.5](https://github.com/LerianStudio/midaz/compare/v1.48.0-beta.4...v1.48.0-beta.5) (2025-02-14)


### Features

* certificate the env version ([7415f49](https://github.com/LerianStudio/midaz/commit/7415f49e01387e0b4f495283d1ba585cd16e822a))
* change structure ([6036bb8](https://github.com/LerianStudio/midaz/commit/6036bb8eef6b1828fe72f45ea92037b47fb73f91))
* change structure of file ([488eebd](https://github.com/LerianStudio/midaz/commit/488eebd5fb7537d41823e28860a9e2056a29928f))
* change the env ([120f768](https://github.com/LerianStudio/midaz/commit/120f7687ee10343396d52e857842eadc6c5241e9))
* change the ENV VARS to test the flow ([7fb001b](https://github.com/LerianStudio/midaz/commit/7fb001bbfbe38da9fe7a5c1a98e0958e0a8a0bc2))
* change the flow ([ff2a573](https://github.com/LerianStudio/midaz/commit/ff2a573546ec453d241a867347d340cf8e241a98))
* change the flow to track all the folders on components ([c0e343d](https://github.com/LerianStudio/midaz/commit/c0e343d5184882e9a6125507ba0b655993bba910))
* change the LOG_LEVEL NEV to test ([8205ef8](https://github.com/LerianStudio/midaz/commit/8205ef8b6363eb63908b4ffb7bef99aeddbab1d6))
* change the structure of file ([b6441d0](https://github.com/LerianStudio/midaz/commit/b6441d03504a477d54eb71d161c22d7f3a994c48))
* change the trigger to pull request to branch main ([6265a7d](https://github.com/LerianStudio/midaz/commit/6265a7d5011e44c6e45d262d693c92d978b97f50))
* change the value of VAR to test ([727248c](https://github.com/LerianStudio/midaz/commit/727248c327db4fd413adba6564fc5a5b4d304c4d))
* change the VAR version to test ([5cd3cc2](https://github.com/LerianStudio/midaz/commit/5cd3cc24dc6a397e9c536d4b2ade32fc38e74393))
* change the version ([f07ff76](https://github.com/LerianStudio/midaz/commit/f07ff76799a9d19997bbfd5641d138cd020d1d47))
* change the while to validate for ([1716a8b](https://github.com/LerianStudio/midaz/commit/1716a8bca178c290fbec137c80b1cc2532a246af))
* changing the VERSION VAR ([8a7d7e9](https://github.com/LerianStudio/midaz/commit/8a7d7e99eff4db2d28996919a9139734c68219fb))
* check changing the env version ([22d45a7](https://github.com/LerianStudio/midaz/commit/22d45a73462ed5f490c1b38917ce473eb9411721))
* check difference between files ([5fc7bdc](https://github.com/LerianStudio/midaz/commit/5fc7bdcec0e73b442e4c90f36df8df8441bc2780))
* check the history of commits ([b588038](https://github.com/LerianStudio/midaz/commit/b58803873c435584f15523ecd620076287f03f9a))
* check version ([98f6c9e](https://github.com/LerianStudio/midaz/commit/98f6c9e608b8e79168c95cdb8075cf08c89d7264))
* checking the version field ([7af2a00](https://github.com/LerianStudio/midaz/commit/7af2a00313dddfd09929ce7c388d53f8528e0f39))
* clarify the ids of steps on the flow ([b9c6df9](https://github.com/LerianStudio/midaz/commit/b9c6df96449b80ad550483fbe1efd199f6c186fc))
* close the while for ([ac183db](https://github.com/LerianStudio/midaz/commit/ac183dba75b0c4b3905440b99e8fa84e51303b16))
* exclude the version VAR ([acfd652](https://github.com/LerianStudio/midaz/commit/acfd6528ca34cbf71ce234b67e873a0bf2d25fc4))
* exclude the VERSION var ([391516c](https://github.com/LerianStudio/midaz/commit/391516c0890f8caed580ef7f81059c5dd437a412))
* execute the test to notification ([f26f3e0](https://github.com/LerianStudio/midaz/commit/f26f3e0ee80714178fc9f405bb9cbd0b26296c8b))
* force to execute env var prs ([9dda835](https://github.com/LerianStudio/midaz/commit/9dda835dc1f4d1fcf0b2fc57c708649cceee3ae8))
* insert flow notification on ENV VARS changes ([6388b47](https://github.com/LerianStudio/midaz/commit/6388b47b2a2972dc3522dcfc14d29ce553ab840b))
* insert git fetch prune on flow ([645f0f9](https://github.com/LerianStudio/midaz/commit/645f0f970c8c4557922935bb24c039ea5f505110))
* maintain ledger folder ([d386e0b](https://github.com/LerianStudio/midaz/commit/d386e0b779f2d1aaf73803f200c8bf71a603bafd))
* mantain the correct version on audit .env.example ([91921f8](https://github.com/LerianStudio/midaz/commit/91921f875d72ca51711d740bd77470a022cec5af))
* return the values to default ([87187f0](https://github.com/LerianStudio/midaz/commit/87187f09de47d3fc67fcb2c604d4067cff545f00))
* set the changes on file ([91200b9](https://github.com/LerianStudio/midaz/commit/91200b9e778e985cd5c53bb19acdb388d5bc4b9e))
* set the changes on file ([11a5b75](https://github.com/LerianStudio/midaz/commit/11a5b755d81842d47b5e9e41ba80ab8bb0663793))
* set the changes on file ([964d5d5](https://github.com/LerianStudio/midaz/commit/964d5d53f9c5f4169ef814e494ff31ae52589fb4))
* set the command to compare and verify ([fdcfab0](https://github.com/LerianStudio/midaz/commit/fdcfab0165fee5bdf218abb52df9d9c925462a9b))
* set the compare ([e432447](https://github.com/LerianStudio/midaz/commit/e4324476eab36832b1677cd3ab2539f195542275))
* set the comparison of commits ([e5f9c1a](https://github.com/LerianStudio/midaz/commit/e5f9c1abeb0b34422e7ba054346523d0669fa45a))
* set the configuration ([357bfa6](https://github.com/LerianStudio/midaz/commit/357bfa6993b624794ee1bbd459ca7afbdaa549e4))
* set the configuration and tests ([b3d1361](https://github.com/LerianStudio/midaz/commit/b3d1361413bc07ce277bf9293dabb4992a42fea6))
* set the configuration of message on slack ([197aad1](https://github.com/LerianStudio/midaz/commit/197aad1a2a486488eb5c9f9aa91d2d4fda2fee40))
* set the current version ([3cda634](https://github.com/LerianStudio/midaz/commit/3cda63454ee7b1cd82da62a0226f0d8ab5fd45aa))
* set the diff of versions ([6cf4205](https://github.com/LerianStudio/midaz/commit/6cf4205d976842293edc7e1ee0e4adf6436a3ad0))
* set the env vars ([2b2da89](https://github.com/LerianStudio/midaz/commit/2b2da8970e82338f78aa6b8effca3219ac42b6f1))
* set the estructure verification ([ac5a974](https://github.com/LerianStudio/midaz/commit/ac5a974e4dc77d2dfa073abbd2b90413fa923afe))
* set the files to compare ([d63d071](https://github.com/LerianStudio/midaz/commit/d63d0713d770f724c9da8827e96632a6f53f0f64))
* set the flow of changes ([edd5561](https://github.com/LerianStudio/midaz/commit/edd55612e01451b4734c5fa55df2c79b404467ca))
* set the flow to test file ([65e551f](https://github.com/LerianStudio/midaz/commit/65e551f9d013cc23d27618bee84977eafad718e4))
* set the flow verifying branch ([592d3ad](https://github.com/LerianStudio/midaz/commit/592d3ad981c6d305720742137096e6503a5762fe))
* set the identation ([3776939](https://github.com/LerianStudio/midaz/commit/37769390cf56289d45fff2a16cf9ba87c13fb327))
* set the regex ([fccf47f](https://github.com/LerianStudio/midaz/commit/fccf47f7c037b49b2bc3add4a70546ba950e4daf))
* set the structure ([bf0c621](https://github.com/LerianStudio/midaz/commit/bf0c621d1f864f32058ba4fd3361b769e4aa7ca7))
* set the structure of envs ([838164c](https://github.com/LerianStudio/midaz/commit/838164c196d8da7fa7068a6ad78228f9c1b0d0a9))
* set the structure of github action workflow ([8823bf1](https://github.com/LerianStudio/midaz/commit/8823bf1764afff4759b6e836cd18842e3cefc98e))
* set the structure of github actions ([5fbff7d](https://github.com/LerianStudio/midaz/commit/5fbff7def3e556bce04f379a60456a7ffadbe4a4))
* set the value of log level ([8302bae](https://github.com/LerianStudio/midaz/commit/8302bae876863cf066face808213217cf2abc6cd))
* set the var of audit to test ([e484c49](https://github.com/LerianStudio/midaz/commit/e484c49d403bb7709ff27d457e6389957867aaa0))
* set the verification exclude version ([2a18364](https://github.com/LerianStudio/midaz/commit/2a183647ac5824b55976f1ac32cb4a7078eda6c5))
* set the verification on VERSION var ([e1eaf0c](https://github.com/LerianStudio/midaz/commit/e1eaf0cc01358466b9e2dd7b5848bf93978acddb))
* set the version ([009d101](https://github.com/LerianStudio/midaz/commit/009d1010860a79cc9d705e36262543436ebe3aef))
* set the version ([1881dc0](https://github.com/LerianStudio/midaz/commit/1881dc0b1f8388ed45a3676d9a12d4e47f70368c))
* set the version ([e673903](https://github.com/LerianStudio/midaz/commit/e6739034d71bc49d8eff4c5e428407762eac1a9e))
* set the version ([d07d7d5](https://github.com/LerianStudio/midaz/commit/d07d7d57b2ddd89a71e579001ecd089f0b7bd75a))
* set the version of file ([71ed8f7](https://github.com/LerianStudio/midaz/commit/71ed8f7fc68ac6cbbc55fcadb91d8822597b6fc5))
* set the version to test ([22f7dab](https://github.com/LerianStudio/midaz/commit/22f7dab90d94f62ce9e206edb96c663d92ae1286))
* set the versions ([e4f1c0e](https://github.com/LerianStudio/midaz/commit/e4f1c0efa02a1c14f720f6c05c95d77c2d4b813a))
* set version of file ([a5247e3](https://github.com/LerianStudio/midaz/commit/a5247e3762ad204e0f1e4fac469dcc040f9e4e9d))
* simulate the app bot to increase VERSION var ([e9f97aa](https://github.com/LerianStudio/midaz/commit/e9f97aa06222f234bdc202a3bd432cfdf0861269))
* test changing the log level VAR ([4020a32](https://github.com/LerianStudio/midaz/commit/4020a329626b0121cad4471cf535935ae05ef813))
* test the change on env. example files ([97d2c95](https://github.com/LerianStudio/midaz/commit/97d2c950a35ec7c87e0fc878f21820f2dd0107ab))
* test the execution flow notificate env vars changes ([a1b6b07](https://github.com/LerianStudio/midaz/commit/a1b6b07ce771e5ba0c00d7b39e0d2207667433a2))
* test the structure ([2701552](https://github.com/LerianStudio/midaz/commit/27015523966dbb37c5289b1166bdc871d27dc9d1))
* test the VERSION variable ([7e81a5a](https://github.com/LerianStudio/midaz/commit/7e81a5ad18f61e5c7defbeb7ef08ac03cf11fbdd))
* test the workflow changing ENV VARS on scenario test ([a3e8552](https://github.com/LerianStudio/midaz/commit/a3e855240a04e680812e8a9cd9d8e5dac129b6b3))
* the the flow to notify devops team ([847b4b2](https://github.com/LerianStudio/midaz/commit/847b4b22b926304ab57f130f447d09ff1285db47))
* using identation ([9e88384](https://github.com/LerianStudio/midaz/commit/9e88384bc1e224df361bf02b18b0f3ccb013d85e))
* verify current branch ([59ef127](https://github.com/LerianStudio/midaz/commit/59ef1278fe96b193586dd12e100e40670325d077))
* verify env. example changes ([82a0926](https://github.com/LerianStudio/midaz/commit/82a09261f00e734813d38c750887809f739dd798))
* verify the change of the flow ([c7ca86e](https://github.com/LerianStudio/midaz/commit/c7ca86ed2dd9e8264e23de608d719c0fc4a28530))
* verify the changes ([d85473b](https://github.com/LerianStudio/midaz/commit/d85473bd324bc5d40f8ae55de80c3f67b9b9127d))
* verify version ([f22eec0](https://github.com/LerianStudio/midaz/commit/f22eec0787cf1b5f3c8ba32ef9bfd98448a55407))
* verify version ([3eabb84](https://github.com/LerianStudio/midaz/commit/3eabb84fe5e252b21ba4155bf5fb0f45be22395b))
* verify VERSION ([c9e44dc](https://github.com/LerianStudio/midaz/commit/c9e44dce8d9bf3b93594f698c863b432b43d5b07))


### Bug Fixes

* change reference name ledger to onboarding; :bug: :bug: ([2568d2b](https://github.com/LerianStudio/midaz/commit/2568d2b313b8cabfefef36bebc74473117b5827d))

## [1.48.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.48.0-beta.3...v1.48.0-beta.4) (2025-02-13)


### Bug Fixes

* make name required when creating asset :bug: ([58f7fd6](https://github.com/LerianStudio/midaz/commit/58f7fd6bd39359ad26e56b3392c3a5a6c145bcc9))

## [1.48.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.48.0-beta.2...v1.48.0-beta.3) (2025-02-11)


### Features

* add devops to codeowner when change .env.example files; :sparkles: ([e1328f8](https://github.com/LerianStudio/midaz/commit/e1328f8117f68ecebad7f5fe65a843ace78d30ab))


### Bug Fixes

* add dev too to validate pr in .envs :bug: ([0aae153](https://github.com/LerianStudio/midaz/commit/0aae153ca9ce3665fea4bd3317111a15d58f595b))

## [1.48.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.48.0-beta.1...v1.48.0-beta.2) (2025-02-11)


### Bug Fixes

* update golang dependencies based on dependabot; :bug: ([84af1ce](https://github.com/LerianStudio/midaz/commit/84af1ce6da5ccc9a54e78d09ff7b41388d4684a3))
* update swagger and openapi; :bug: ([d034c4e](https://github.com/LerianStudio/midaz/commit/d034c4e36a4c9994fab4cb43a023d2244b5142b3))

## [1.48.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.47.0...v1.48.0-beta.1) (2025-02-10)


### Features

* add new table balance for perfomance poc; ([cd1bc6b](https://github.com/LerianStudio/midaz/commit/cd1bc6b846615b1d8997dadc6e1887c238e82e6b))
* add on asset and account when creation account to send to the transaction balance queue; :sparkles: ([aeedcf9](https://github.com/LerianStudio/midaz/commit/aeedcf9ef366bd9e7e8f11ecb9b675c2cfc3438f))
* add trace on casdoor; :sparkles: ([63d084d](https://github.com/LerianStudio/midaz/commit/63d084d473484e7585e4c22c48675b8100c2de53))
* create balance sql implements; ([e89d870](https://github.com/LerianStudio/midaz/commit/e89d870a7c4c6b42a77fd0331889cbd061dc43db))
* create structure to receive account from rabbitmq and create balance; :sparkles: ([6a3b41c](https://github.com/LerianStudio/midaz/commit/6a3b41c847f8de3e25f146040e8c9d43d0d3390f))
* update balance async; rename validate accounts to validate balance rules; :sparkles: ([587f362](https://github.com/LerianStudio/midaz/commit/587f3622bcfdc8f23432cec3d219abc6c5fdc5ce))


### Bug Fixes

* add column that accepts account to be negative; :bug: ([e952a37](https://github.com/LerianStudio/midaz/commit/e952a379a3236c56dc739be72e5e8909e5b3a1a6))
* add insert values on table; :bug: ([e23a9ab](https://github.com/LerianStudio/midaz/commit/e23a9abefe9ccdbe6d866e69beb2fc250b606f42))
* add log to catch erros when cannot marshal result var; :bug: ([1074626](https://github.com/LerianStudio/midaz/commit/10746263fffb4296cb08784b8d617196397e5130))
* add rule to only lock balances on redis if has more than one balance on slice; :bug: ([1c52e66](https://github.com/LerianStudio/midaz/commit/1c52e66c12e44d5c2451ca1c8fc484cb6054b213))
* add two index one by alias e another by account_id; :bug: ([f6b36ab](https://github.com/LerianStudio/midaz/commit/f6b36ab32c777156d94aed68add262592fd05bc2))
* add validation to avoid ambiguous account on source and distribute; :bug: ([d201dec](https://github.com/LerianStudio/midaz/commit/d201dec63c9c6fbf91a8b03dac0aea45a247ccd8))
* adjust accounts removing old fields and reorganizing it; :bug: ([b86d541](https://github.com/LerianStudio/midaz/commit/b86d5411309ecb7116ea3305a048cc3592021b42))
* adjust go test after change asset and account; :bug: :bug: ([6bddbea](https://github.com/LerianStudio/midaz/commit/6bddbea6ac442f9a76276674c64f881ce7bad7ca))
* make sec and make lint; :bug: ([3eacf02](https://github.com/LerianStudio/midaz/commit/3eacf0274b25cedfb1714d771dbbd3e98fa7ffcb))
* midazId name; :bug: ([263cae3](https://github.com/LerianStudio/midaz/commit/263cae37cb5b5f308f1aaff64255349b8e1d29af))
* remove extension; add index; :bug: ([8d06d22](https://github.com/LerianStudio/midaz/commit/8d06d22f70120932632fdb537f8b05977f71d564))
* remove grpc accounts from ledger; :bug: ([7e91f64](https://github.com/LerianStudio/midaz/commit/7e91f64fa0d6c0442aac2dab8ad869706fb1c500))
* remove old locks rules; :bug: ([2bba68d](https://github.com/LerianStudio/midaz/commit/2bba68d8915b3d78bba77d1735c06fe53f172821))
* remove old references from accounts; :bug: ([dfb0a57](https://github.com/LerianStudio/midaz/commit/dfb0a57d1ed2fabab1e1c71f36c743fc5b61fa89))
* remove portfolio routes and funcs on account that was deprecated and update postman; :bug: ([7bc5f5e](https://github.com/LerianStudio/midaz/commit/7bc5f5e7f46465ef4d7fd415a685b3d9e680593d))
* remove protobuf door on ledger; :bug: ([b3d0b56](https://github.com/LerianStudio/midaz/commit/b3d0b566b2f039a362c51fac351d2c8f843c1efa))
* remove protobuf reference on ledger; :bug: ([1ccefc5](https://github.com/LerianStudio/midaz/commit/1ccefc5f1b15287a8a5c3711f5f73f99e5737382))
* reusable protobuf door on transaction and transaction door on audit; :bug: ([2224904](https://github.com/LerianStudio/midaz/commit/2224904540995a9bb2096dda7c904f4a7b3d4a2c))
* revert telemetry; :bug: ([2d1fbce](https://github.com/LerianStudio/midaz/commit/2d1fbcef56af103077171a8232eda9d48f612849))
* tables accounts and balance; :bug: ([23f0e16](https://github.com/LerianStudio/midaz/commit/23f0e16f63e44e5c07aa6947c2ccdd2e7951f20b))
* update asset to remove old fields on create external account; :bug: ([ef34a89](https://github.com/LerianStudio/midaz/commit/ef34a89818da10f7fe63f5f004f903b402afa4f3))
* update description log oeprations to operations; :bug: ([5fe3156](https://github.com/LerianStudio/midaz/commit/5fe3156b5eb6686ed9bc661fac5e123dee8b5d53))
* update dockerfile port; :bug: ([e3c17a2](https://github.com/LerianStudio/midaz/commit/e3c17a2a443aa38856ab70a0e599db7d923de48c))
* update ports docker; :bug: ([41260c9](https://github.com/LerianStudio/midaz/commit/41260c9d991a6e2b299f79409c5d9fb132596ff2))

## [1.47.0](https://github.com/LerianStudio/midaz/compare/v1.46.0...v1.47.0) (2025-02-10)


### Bug Fixes

* add env workflows ([902fc29](https://github.com/LerianStudio/midaz/commit/902fc294bc35bb16dca76435be6939ad17f0e558))
* change make up to make all-services COMMAND="up" ([10ecb01](https://github.com/LerianStudio/midaz/commit/10ecb01efbdc14952592df89b86432fd3422cc24))
* change this job to don't run if came from a fork. ([abb0628](https://github.com/LerianStudio/midaz/commit/abb0628d210017923713ec0f62cb3e08867c4911))
* change this job to don't run this step if came for a fork. ([7a6714b](https://github.com/LerianStudio/midaz/commit/7a6714b1dc11f5e2c3a41754c70ad93d8632fe42))
* midaz test :bug: ([46f6111](https://github.com/LerianStudio/midaz/commit/46f6111e9d47a5be4e176c4dea91eef34a6c9807))
* remove env fork-workflows ([290630b](https://github.com/LerianStudio/midaz/commit/290630ba3c6de8310d35b342eed6ea86ad2485fb))
* remove make up from general make; ([64590ea](https://github.com/LerianStudio/midaz/commit/64590eade120d5b1c69f2eaaf03fbd5a6098fb18))
* revert pull_request ([cee4228](https://github.com/LerianStudio/midaz/commit/cee42285a3264fa93bfa8a21f109a89f9ac1fcf9))
* rollback all changes ([4cf665b](https://github.com/LerianStudio/midaz/commit/4cf665bc1a4405c83057129eca40d23cff08e4ec))
* rollback commands; ([79f1525](https://github.com/LerianStudio/midaz/commit/79f1525fb127bf1f7e27f09c8f6778829664d2d2))
* rollback first change ([fe3e296](https://github.com/LerianStudio/midaz/commit/fe3e296796339d2c07debd9bf653a1dc21c8c252))
* test pull_request_target to try to run action on midaz principal context ([38be5d9](https://github.com/LerianStudio/midaz/commit/38be5d95569f37bc8f163b51396f74570cc8e158))
* update docs,  swagger and open apis files; :bug: ([768a857](https://github.com/LerianStudio/midaz/commit/768a857995538ae7876c0f4da24c5781e756d16b))
* update go mod and sum; :bug: ([ecdec76](https://github.com/LerianStudio/midaz/commit/ecdec7648bcd5eb6c865868775f2c2a411f4b7ae))
* update libs in go mod and go sum; :bug: ([c83e52a](https://github.com/LerianStudio/midaz/commit/c83e52a8c13c2baad585d289013d4a476a080dc4))

## [1.47.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.47.0-beta.2...v1.47.0-beta.3) (2025-01-31)


### Bug Fixes

* midaz test :bug: ([46f6111](https://github.com/LerianStudio/midaz/commit/46f6111e9d47a5be4e176c4dea91eef34a6c9807))
* update docs,  swagger and open apis files; :bug: ([768a857](https://github.com/LerianStudio/midaz/commit/768a857995538ae7876c0f4da24c5781e756d16b))
* update go mod and sum; :bug: ([ecdec76](https://github.com/LerianStudio/midaz/commit/ecdec7648bcd5eb6c865868775f2c2a411f4b7ae))
* update libs in go mod and go sum; :bug: ([c83e52a](https://github.com/LerianStudio/midaz/commit/c83e52a8c13c2baad585d289013d4a476a080dc4))

## [1.47.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.47.0-beta.1...v1.47.0-beta.2) (2025-01-30)

## [1.47.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.46.0...v1.47.0-beta.1) (2025-01-29)


### Bug Fixes

* add env workflows ([902fc29](https://github.com/LerianStudio/midaz/commit/902fc294bc35bb16dca76435be6939ad17f0e558))
* change make up to make all-services COMMAND="up" ([10ecb01](https://github.com/LerianStudio/midaz/commit/10ecb01efbdc14952592df89b86432fd3422cc24))
* change this job to don't run if came from a fork. ([abb0628](https://github.com/LerianStudio/midaz/commit/abb0628d210017923713ec0f62cb3e08867c4911))
* change this job to don't run this step if came for a fork. ([7a6714b](https://github.com/LerianStudio/midaz/commit/7a6714b1dc11f5e2c3a41754c70ad93d8632fe42))
* remove env fork-workflows ([290630b](https://github.com/LerianStudio/midaz/commit/290630ba3c6de8310d35b342eed6ea86ad2485fb))
* remove make up from general make; ([64590ea](https://github.com/LerianStudio/midaz/commit/64590eade120d5b1c69f2eaaf03fbd5a6098fb18))
* revert pull_request ([cee4228](https://github.com/LerianStudio/midaz/commit/cee42285a3264fa93bfa8a21f109a89f9ac1fcf9))
* rollback all changes ([4cf665b](https://github.com/LerianStudio/midaz/commit/4cf665bc1a4405c83057129eca40d23cff08e4ec))
* rollback commands; ([79f1525](https://github.com/LerianStudio/midaz/commit/79f1525fb127bf1f7e27f09c8f6778829664d2d2))
* rollback first change ([fe3e296](https://github.com/LerianStudio/midaz/commit/fe3e296796339d2c07debd9bf653a1dc21c8c252))
* test pull_request_target to try to run action on midaz principal context ([38be5d9](https://github.com/LerianStudio/midaz/commit/38be5d95569f37bc8f163b51396f74570cc8e158))

## [1.46.0](https://github.com/LerianStudio/midaz/compare/v1.45.0...v1.46.0) (2025-01-24)


### Features

* add account_alias and account_external_brl on postman; :sparkles: ([cc20914](https://github.com/LerianStudio/midaz/commit/cc209146430f75c6032e361f83b7b5312ad28e17))
* add user midaz non root to help to send cpu and mem to grafana; :sparkles: ([e6e30ad](https://github.com/LerianStudio/midaz/commit/e6e30ad0416f6e477ff0b4a1c00f76d294bf3ae3))
* warning about customize .env variables to fit your environment; add more colors; :sparkles: ([55874b5](https://github.com/LerianStudio/midaz/commit/55874b50eabb591da363d3015b008f61e89b1de9))


### Bug Fixes

* add mongo uri variable; add rabbit uri variable; :bug: ([5af9415](https://github.com/LerianStudio/midaz/commit/5af9415ff706cb1820401c8482373df98d27533c))
* add PortfolioID to account update functionality :sparkles: :sparkles: :bug: :bug: ([ad04b2b](https://github.com/LerianStudio/midaz/commit/ad04b2bf79d735f0d1fd73c9e545b8e999f72d35))
* add slice make init to avoid panic and valida len os slice before calculate cursor :bug: ([b67bec7](https://github.com/LerianStudio/midaz/commit/b67bec78ec3e16a11e9e6451f97d3f835d67e10f))
* add some normalize to docker :bug: ([b49825d](https://github.com/LerianStudio/midaz/commit/b49825de06567695b5c7feb723e343fa62a638d5))
* add sources, destinations and operations to get transactions; :bug: ([f9afbef](https://github.com/LerianStudio/midaz/commit/f9afbefd72126dd665da782aa9ce794754907248))
* go lint :bug: ([ee0ea3f](https://github.com/LerianStudio/midaz/commit/ee0ea3f5c6c83041662118a5a9931539cae2e867))
* grafana final adjusts to always open asking for login :bug: ([2363b63](https://github.com/LerianStudio/midaz/commit/2363b63c4d3c25ee64f819c6cd2c4108f5bee48c))
* implements login screen to grafana otel :bug: ([3ce2669](https://github.com/LerianStudio/midaz/commit/3ce2669d078a1ba68f17b8d51a178eae5c6fbcff))
* remove command to broken grafana :bug: ([6421ab1](https://github.com/LerianStudio/midaz/commit/6421ab16285e7247a62f3c107edc7300cd8e642b))
* remove from outside metadata validation :bug: ([d9cebc0](https://github.com/LerianStudio/midaz/commit/d9cebc0c4824a24d71229fd22f3341e8ab2fc0b6))
* remove unused air files :bug: ([6ef5a7f](https://github.com/LerianStudio/midaz/commit/6ef5a7f0782487a3461b81f54bff2c4fbe0b4825))
* rename personalized midaz headers :bug: ([b891e6e](https://github.com/LerianStudio/midaz/commit/b891e6e6c853c1a766e59616e5ea452e8619849a))
* rename personalized midaz headers. :bug: ([847e9ff](https://github.com/LerianStudio/midaz/commit/847e9ff84d3535e618003bc0ccd35f03adf183ed))
* run grafana with anonymous auth disabled :bug: ([44efa74](https://github.com/LerianStudio/midaz/commit/44efa7417bad6fa2e90ad6b16d2a09ff698daa55))
* treatment for old transactions without transaction body :bug: ([129f9de](https://github.com/LerianStudio/midaz/commit/129f9de7b0a62903d14738c379b47f4def41c844))
* update allow sending and receiving fields from accounts :bug: ([821a70d](https://github.com/LerianStudio/midaz/commit/821a70db7e4de425561646be5ee919b3deefbb34))
* update dependencies by dependabot :bug: ([b822c8d](https://github.com/LerianStudio/midaz/commit/b822c8d0371520cccd4d6782c75c577b6369315a))
* update name of midaz-full to midaz-stack :bug: ([9c63cf1](https://github.com/LerianStudio/midaz/commit/9c63cf1da9363a2ef7554bad5d0f8ae4548885d8))

## [1.46.0-beta.5](https://github.com/LerianStudio/midaz/compare/v1.46.0-beta.4...v1.46.0-beta.5) (2025-01-24)


### Bug Fixes

* add PortfolioID to account update functionality :sparkles: :sparkles: :bug: :bug: ([ad04b2b](https://github.com/LerianStudio/midaz/commit/ad04b2bf79d735f0d1fd73c9e545b8e999f72d35))

## [1.46.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.46.0-beta.3...v1.46.0-beta.4) (2025-01-23)


### Bug Fixes

* add slice make init to avoid panic and valida len os slice before calculate cursor :bug: ([b67bec7](https://github.com/LerianStudio/midaz/commit/b67bec78ec3e16a11e9e6451f97d3f835d67e10f))

## [1.46.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.46.0-beta.2...v1.46.0-beta.3) (2025-01-23)


### Bug Fixes

* add sources, destinations and operations to get transactions; :bug: ([f9afbef](https://github.com/LerianStudio/midaz/commit/f9afbefd72126dd665da782aa9ce794754907248))
* go lint :bug: ([ee0ea3f](https://github.com/LerianStudio/midaz/commit/ee0ea3f5c6c83041662118a5a9931539cae2e867))
* remove command to broken grafana :bug: ([6421ab1](https://github.com/LerianStudio/midaz/commit/6421ab16285e7247a62f3c107edc7300cd8e642b))
* remove from outside metadata validation :bug: ([d9cebc0](https://github.com/LerianStudio/midaz/commit/d9cebc0c4824a24d71229fd22f3341e8ab2fc0b6))

## [1.46.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.46.0-beta.1...v1.46.0-beta.2) (2025-01-22)


### Features

* add account_alias and account_external_brl on postman; :sparkles: ([cc20914](https://github.com/LerianStudio/midaz/commit/cc209146430f75c6032e361f83b7b5312ad28e17))
* add user midaz non root to help to send cpu and mem to grafana; :sparkles: ([e6e30ad](https://github.com/LerianStudio/midaz/commit/e6e30ad0416f6e477ff0b4a1c00f76d294bf3ae3))
* warning about customize .env variables to fit your environment; add more colors; :sparkles: ([55874b5](https://github.com/LerianStudio/midaz/commit/55874b50eabb591da363d3015b008f61e89b1de9))


### Bug Fixes

* add mongo uri variable; add rabbit uri variable; :bug: ([5af9415](https://github.com/LerianStudio/midaz/commit/5af9415ff706cb1820401c8482373df98d27533c))
* add some normalize to docker :bug: ([b49825d](https://github.com/LerianStudio/midaz/commit/b49825de06567695b5c7feb723e343fa62a638d5))
* grafana final adjusts to always open asking for login :bug: ([2363b63](https://github.com/LerianStudio/midaz/commit/2363b63c4d3c25ee64f819c6cd2c4108f5bee48c))
* implements login screen to grafana otel :bug: ([3ce2669](https://github.com/LerianStudio/midaz/commit/3ce2669d078a1ba68f17b8d51a178eae5c6fbcff))
* remove unused air files :bug: ([6ef5a7f](https://github.com/LerianStudio/midaz/commit/6ef5a7f0782487a3461b81f54bff2c4fbe0b4825))
* rename personalized midaz headers :bug: ([b891e6e](https://github.com/LerianStudio/midaz/commit/b891e6e6c853c1a766e59616e5ea452e8619849a))
* rename personalized midaz headers. :bug: ([847e9ff](https://github.com/LerianStudio/midaz/commit/847e9ff84d3535e618003bc0ccd35f03adf183ed))
* run grafana with anonymous auth disabled :bug: ([44efa74](https://github.com/LerianStudio/midaz/commit/44efa7417bad6fa2e90ad6b16d2a09ff698daa55))
* update allow sending and receiving fields from accounts :bug: ([821a70d](https://github.com/LerianStudio/midaz/commit/821a70db7e4de425561646be5ee919b3deefbb34))
* update dependencies by dependabot :bug: ([b822c8d](https://github.com/LerianStudio/midaz/commit/b822c8d0371520cccd4d6782c75c577b6369315a))
* update name of midaz-full to midaz-stack :bug: ([9c63cf1](https://github.com/LerianStudio/midaz/commit/9c63cf1da9363a2ef7554bad5d0f8ae4548885d8))

## [1.46.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.45.0...v1.46.0-beta.1) (2025-01-20)


### Bug Fixes

* treatment for old transactions without transaction body :bug: ([129f9de](https://github.com/LerianStudio/midaz/commit/129f9de7b0a62903d14738c379b47f4def41c844))

## [1.45.0](https://github.com/LerianStudio/midaz/compare/v1.44.0...v1.45.0) (2025-01-17)


### Features

* add api on postman; adjust lint; generate swagger and open api; :sparkles: ([095f2d6](https://github.com/LerianStudio/midaz/commit/095f2d6942e7d8ab6be0822e66509d6d65392a10))
* add go routines to update; some postgres configs :sparkles: ([25fdb70](https://github.com/LerianStudio/midaz/commit/25fdb706ef65ec550172bb7f6d47652eb8f944f5))
* add logger :sparkles: ([6ff156d](https://github.com/LerianStudio/midaz/commit/6ff156d268d5647f19c9bcae394a5e788fddac4b))
* add magic numbers to constant :sparkles: ([acc57a3](https://github.com/LerianStudio/midaz/commit/acc57a3fc91ebbe4d4a7492bbd4bd49d23945e78))
* add optimistic lock on database using version to control race condition; ([3f37ade](https://github.com/LerianStudio/midaz/commit/3f37adeb3592531aa64612915066998defa00c06))
* add transaction body on database; :sparkles: ([d5b6197](https://github.com/LerianStudio/midaz/commit/d5b619788782563d2c336fccaa01f4584ac54e57))
* added router find account by alias :sparkles: ([a2e8c99](https://github.com/LerianStudio/midaz/commit/a2e8c99ec816149c23beb5962a600ca7d7bb0328))
* adjust time :sparkles: ([60da1cf](https://github.com/LerianStudio/midaz/commit/60da1cf7dce25469c87bf5786aa6b603fbe79638))
* create race condition using gorotine and chanel ([3248ee7](https://github.com/LerianStudio/midaz/commit/3248ee7e8fca50dc8882327e94f4c9fcbfd3529e))
* final adjusts to rever transaction :sparkles: ([44b650c](https://github.com/LerianStudio/midaz/commit/44b650cb66cf14773119b7b94c790fa61a7e6231))
* new implementatios; :sparkles: ([a8f5a6d](https://github.com/LerianStudio/midaz/commit/a8f5a6deb065858eb90a3a9b74c641ecc304e4f5))
* new race condition implementation ([6bb89dd](https://github.com/LerianStudio/midaz/commit/6bb89dd46e163b057cb4c7c32cdd8e3a8c418147))
* new updates to avoid race condition ([97448dc](https://github.com/LerianStudio/midaz/commit/97448dc69d6bcfafe66bbd94d81cff8b4733da3e))
* push choco :sparkles: ([4d19380](https://github.com/LerianStudio/midaz/commit/4d19380685d0bad020ed4f0b67e73dcac372876e))
* update time lock :sparkles: ([beb4921](https://github.com/LerianStudio/midaz/commit/beb49216d8e7c7ddcc76a841c9c454304abd0e62))


### Bug Fixes

* add defer rollback :bug: ([0cdabd1](https://github.com/LerianStudio/midaz/commit/0cdabd13e58d91d6f86170c70baf4d602690bd16))
* add revert logic to object :bug: ([e0d36ad](https://github.com/LerianStudio/midaz/commit/e0d36ad33bb4962d4a7b4fc6646f5750e842a8f8))
* add rollback in case of error to unlock database; :bug: ([66d7416](https://github.com/LerianStudio/midaz/commit/66d74168b2da61b5fc74662ff7150403fb624b36))
* add unlock :bug: ([0c62a31](https://github.com/LerianStudio/midaz/commit/0c62a314e4ad86918b6955ba3792ce2017102c8e))
* adjust to remove lock of get accounts :bug: ([1ddf09f](https://github.com/LerianStudio/midaz/commit/1ddf09f939da41d4ebabfd339b00d7caf9dc29f6))
* change place :bug: ([3970a04](https://github.com/LerianStudio/midaz/commit/3970a04dd1ac5157081815726766387434ad0b66))
* improve idempotency using setnx; :bug: ([5a7988e](https://github.com/LerianStudio/midaz/commit/5a7988e161e0bb64a64149c1871ba2f0c9f2dbd5))
* lint; add version; :bug: ([a7df566](https://github.com/LerianStudio/midaz/commit/a7df566659892e15eddc0486087d93a74cf707d4))
* make lint :bug: ([ff3a8ad](https://github.com/LerianStudio/midaz/commit/ff3a8ad792b1b592fb3faa93bcd86dc4f45a1572))
* merge with develop :bug: ([a04a8ee](https://github.com/LerianStudio/midaz/commit/a04a8eebeda62ff6f1812606c778aa5bcbf15041))
* reduce lock time :bug: ([ddb6a60](https://github.com/LerianStudio/midaz/commit/ddb6a60ca16b3560d6b8c0802e12fb9a246894bf))
* unlock specify by get accounts :bug: ([26af469](https://github.com/LerianStudio/midaz/commit/26af4697aff95095a98681af98a3c0658a60c75b))
* update go mod dependabot :bug: ([f776fcc](https://github.com/LerianStudio/midaz/commit/f776fcc678cbbb652dfb01ff6ab7890b6ac85777))
* updates to improve race condition :bug: ([022c3c9](https://github.com/LerianStudio/midaz/commit/022c3c90f6827149bc8ba4e78b2acb314895bbc8))

## [1.45.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.45.0-beta.2...v1.45.0-beta.3) (2025-01-17)


### Features

* add api on postman; adjust lint; generate swagger and open api; :sparkles: ([095f2d6](https://github.com/LerianStudio/midaz/commit/095f2d6942e7d8ab6be0822e66509d6d65392a10))
* add transaction body on database; :sparkles: ([d5b6197](https://github.com/LerianStudio/midaz/commit/d5b619788782563d2c336fccaa01f4584ac54e57))
* final adjusts to rever transaction :sparkles: ([44b650c](https://github.com/LerianStudio/midaz/commit/44b650cb66cf14773119b7b94c790fa61a7e6231))
* new implementatios; :sparkles: ([a8f5a6d](https://github.com/LerianStudio/midaz/commit/a8f5a6deb065858eb90a3a9b74c641ecc304e4f5))


### Bug Fixes

* add revert logic to object :bug: ([e0d36ad](https://github.com/LerianStudio/midaz/commit/e0d36ad33bb4962d4a7b4fc6646f5750e842a8f8))

## [1.45.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.45.0-beta.1...v1.45.0-beta.2) (2025-01-15)


### Features

* add go routines to update; some postgres configs :sparkles: ([25fdb70](https://github.com/LerianStudio/midaz/commit/25fdb706ef65ec550172bb7f6d47652eb8f944f5))
* add logger :sparkles: ([6ff156d](https://github.com/LerianStudio/midaz/commit/6ff156d268d5647f19c9bcae394a5e788fddac4b))
* add magic numbers to constant :sparkles: ([acc57a3](https://github.com/LerianStudio/midaz/commit/acc57a3fc91ebbe4d4a7492bbd4bd49d23945e78))
* add optimistic lock on database using version to control race condition; ([3f37ade](https://github.com/LerianStudio/midaz/commit/3f37adeb3592531aa64612915066998defa00c06))
* adjust time :sparkles: ([60da1cf](https://github.com/LerianStudio/midaz/commit/60da1cf7dce25469c87bf5786aa6b603fbe79638))
* create race condition using gorotine and chanel ([3248ee7](https://github.com/LerianStudio/midaz/commit/3248ee7e8fca50dc8882327e94f4c9fcbfd3529e))
* new race condition implementation ([6bb89dd](https://github.com/LerianStudio/midaz/commit/6bb89dd46e163b057cb4c7c32cdd8e3a8c418147))
* new updates to avoid race condition ([97448dc](https://github.com/LerianStudio/midaz/commit/97448dc69d6bcfafe66bbd94d81cff8b4733da3e))
* update time lock :sparkles: ([beb4921](https://github.com/LerianStudio/midaz/commit/beb49216d8e7c7ddcc76a841c9c454304abd0e62))


### Bug Fixes

* add defer rollback :bug: ([0cdabd1](https://github.com/LerianStudio/midaz/commit/0cdabd13e58d91d6f86170c70baf4d602690bd16))
* add rollback in case of error to unlock database; :bug: ([66d7416](https://github.com/LerianStudio/midaz/commit/66d74168b2da61b5fc74662ff7150403fb624b36))
* add unlock :bug: ([0c62a31](https://github.com/LerianStudio/midaz/commit/0c62a314e4ad86918b6955ba3792ce2017102c8e))
* adjust to remove lock of get accounts :bug: ([1ddf09f](https://github.com/LerianStudio/midaz/commit/1ddf09f939da41d4ebabfd339b00d7caf9dc29f6))
* change place :bug: ([3970a04](https://github.com/LerianStudio/midaz/commit/3970a04dd1ac5157081815726766387434ad0b66))
* improve idempotency using setnx; :bug: ([5a7988e](https://github.com/LerianStudio/midaz/commit/5a7988e161e0bb64a64149c1871ba2f0c9f2dbd5))
* lint; add version; :bug: ([a7df566](https://github.com/LerianStudio/midaz/commit/a7df566659892e15eddc0486087d93a74cf707d4))
* make lint :bug: ([ff3a8ad](https://github.com/LerianStudio/midaz/commit/ff3a8ad792b1b592fb3faa93bcd86dc4f45a1572))
* merge with develop :bug: ([a04a8ee](https://github.com/LerianStudio/midaz/commit/a04a8eebeda62ff6f1812606c778aa5bcbf15041))
* reduce lock time :bug: ([ddb6a60](https://github.com/LerianStudio/midaz/commit/ddb6a60ca16b3560d6b8c0802e12fb9a246894bf))
* unlock specify by get accounts :bug: ([26af469](https://github.com/LerianStudio/midaz/commit/26af4697aff95095a98681af98a3c0658a60c75b))
* update go mod dependabot :bug: ([f776fcc](https://github.com/LerianStudio/midaz/commit/f776fcc678cbbb652dfb01ff6ab7890b6ac85777))
* updates to improve race condition :bug: ([022c3c9](https://github.com/LerianStudio/midaz/commit/022c3c90f6827149bc8ba4e78b2acb314895bbc8))

## [1.45.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.44.1-beta.1...v1.45.0-beta.1) (2025-01-09)


### Features

* added router find account by alias :sparkles: ([a2e8c99](https://github.com/LerianStudio/midaz/commit/a2e8c99ec816149c23beb5962a600ca7d7bb0328))
* push choco :sparkles: ([4d19380](https://github.com/LerianStudio/midaz/commit/4d19380685d0bad020ed4f0b67e73dcac372876e))

## [1.44.1-beta.1](https://github.com/LerianStudio/midaz/compare/v1.44.0...v1.44.1-beta.1) (2025-01-08)

## [1.44.0](https://github.com/LerianStudio/midaz/compare/v1.43.0...v1.44.0) (2025-01-08)


### Features

* add matrix to generate images to ledger, audit and transaction; ([d61fa94](https://github.com/LerianStudio/midaz/commit/d61fa94dd7f0d261250a4d95dc739d88db39d0b4))
* add verification.txt on tools ([7b270e5](https://github.com/LerianStudio/midaz/commit/7b270e5a677bf8e14665d498897a872045a51c75))


### Bug Fixes

* check context if is diff nil :bug: ([8212740](https://github.com/LerianStudio/midaz/commit/821274079acf0278faf69ffb383723d4f8c59349))
* delete from root :bug: ([eefa57a](https://github.com/LerianStudio/midaz/commit/eefa57a61b36b57a66e7244001733cbc32bbc10d))
* update libs; :bug: ([dfb3b1b](https://github.com/LerianStudio/midaz/commit/dfb3b1b0c392cb9f9a569dd349d9db80f49b2acb))
* update some commands; :bug: ([0e21d6a](https://github.com/LerianStudio/midaz/commit/0e21d6a8af8289b9c3c092447eaeee2c392abfdf))

## [1.44.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.44.0-beta.1...v1.44.0-beta.2) (2025-01-08)


### Features

* add verification.txt on tools ([7b270e5](https://github.com/LerianStudio/midaz/commit/7b270e5a677bf8e14665d498897a872045a51c75))


### Bug Fixes

* check context if is diff nil :bug: ([8212740](https://github.com/LerianStudio/midaz/commit/821274079acf0278faf69ffb383723d4f8c59349))
* delete from root :bug: ([eefa57a](https://github.com/LerianStudio/midaz/commit/eefa57a61b36b57a66e7244001733cbc32bbc10d))

## [1.44.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.43.0...v1.44.0-beta.1) (2025-01-07)


### Features

* add matrix to generate images to ledger, audit and transaction; ([d61fa94](https://github.com/LerianStudio/midaz/commit/d61fa94dd7f0d261250a4d95dc739d88db39d0b4))


### Bug Fixes

* update libs; :bug: ([dfb3b1b](https://github.com/LerianStudio/midaz/commit/dfb3b1b0c392cb9f9a569dd349d9db80f49b2acb))
* update some commands; :bug: ([0e21d6a](https://github.com/LerianStudio/midaz/commit/0e21d6a8af8289b9c3c092447eaeee2c392abfdf))

## [1.43.0](https://github.com/LerianStudio/midaz/compare/v1.42.0...v1.43.0) (2025-01-06)


### Bug Fixes

* no file verification :bug: ([aec934b](https://github.com/LerianStudio/midaz/commit/aec934bb9b051bee1dfb6393a776ed0f6c983292))

## [1.43.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.42.0...v1.43.0-beta.1) (2025-01-06)


### Bug Fixes

* no file verification :bug: ([aec934b](https://github.com/LerianStudio/midaz/commit/aec934bb9b051bee1dfb6393a776ed0f6c983292))

## [1.42.0](https://github.com/LerianStudio/midaz/compare/v1.41.0...v1.42.0) (2025-01-02)


### Features

* will query the field alias added field alias for query via query param :sparkles: ([12e869e](https://github.com/LerianStudio/midaz/commit/12e869e2c30db1e98b0495fdd66c53bcd2b39222))


### Bug Fixes

* changed the way to add the key :bug: ([10e7b13](https://github.com/LerianStudio/midaz/commit/10e7b1344a64a36153e2c281485aa92b3e10673b))

## [1.42.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.42.0-beta.1...v1.42.0-beta.2) (2025-01-02)


### Bug Fixes

* changed the way to add the key :bug: ([10e7b13](https://github.com/LerianStudio/midaz/commit/10e7b1344a64a36153e2c281485aa92b3e10673b))

## [1.42.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.41.0...v1.42.0-beta.1) (2024-12-30)


### Features

* will query the field alias added field alias for query via query param :sparkles: ([12e869e](https://github.com/LerianStudio/midaz/commit/12e869e2c30db1e98b0495fdd66c53bcd2b39222))

## [1.41.0](https://github.com/LerianStudio/midaz/compare/v1.40.0...v1.41.0) (2024-12-27)


### Bug Fixes

* final :bug: ([e07f491](https://github.com/LerianStudio/midaz/commit/e07f4912b23046ab2b5ac0bfa8547117bd7f4598))
* formatted version in script install cli :bug: ([d418612](https://github.com/LerianStudio/midaz/commit/d41861296564f2a883f8bbf557169b81bc7e5841))
* name the step wait change update needs of the step from choco :bug: ([c7062e7](https://github.com/LerianStudio/midaz/commit/c7062e79c8b7fec4b39e0bf7fc0a1f8a84b146f0))
* runner to branch develop :bug: ([f1bd475](https://github.com/LerianStudio/midaz/commit/f1bd4756c86aff250e867d5c222d4245db7cd675))
* syntax error replace change to sed bash :bug: ([edbf9c5](https://github.com/LerianStudio/midaz/commit/edbf9c5ccf753f51499a7c0458e2d4a072c575a6))
* test if vars functionals normal :bug: ([be817c0](https://github.com/LerianStudio/midaz/commit/be817c0f95ad4f1f8a6cee1e13d2ccdd53427df1))
* wait 10 minutes before of the exec push to choco :bug: ([0a64bf8](https://github.com/LerianStudio/midaz/commit/0a64bf8d65b85c81a9f8efbc2b59107b17b19a6a))
* wait no runner in develop :bug: ([07883ff](https://github.com/LerianStudio/midaz/commit/07883ff47c7ac29579c23ceb5ae4928cf9ccae86))

## [1.41.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.41.0-beta.1...v1.41.0-beta.2) (2024-12-27)


### Bug Fixes

* wait no runner in develop :bug: ([07883ff](https://github.com/LerianStudio/midaz/commit/07883ff47c7ac29579c23ceb5ae4928cf9ccae86))

## [1.41.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.40.0...v1.41.0-beta.1) (2024-12-27)


### Bug Fixes

* final :bug: ([e07f491](https://github.com/LerianStudio/midaz/commit/e07f4912b23046ab2b5ac0bfa8547117bd7f4598))
* formatted version in script install cli :bug: ([d418612](https://github.com/LerianStudio/midaz/commit/d41861296564f2a883f8bbf557169b81bc7e5841))
* name the step wait change update needs of the step from choco :bug: ([c7062e7](https://github.com/LerianStudio/midaz/commit/c7062e79c8b7fec4b39e0bf7fc0a1f8a84b146f0))
* runner to branch develop :bug: ([f1bd475](https://github.com/LerianStudio/midaz/commit/f1bd4756c86aff250e867d5c222d4245db7cd675))
* syntax error replace change to sed bash :bug: ([edbf9c5](https://github.com/LerianStudio/midaz/commit/edbf9c5ccf753f51499a7c0458e2d4a072c575a6))
* test if vars functionals normal :bug: ([be817c0](https://github.com/LerianStudio/midaz/commit/be817c0f95ad4f1f8a6cee1e13d2ccdd53427df1))
* wait 10 minutes before of the exec push to choco :bug: ([0a64bf8](https://github.com/LerianStudio/midaz/commit/0a64bf8d65b85c81a9f8efbc2b59107b17b19a6a))

## [1.40.0](https://github.com/LerianStudio/midaz/compare/v1.39.0...v1.40.0) (2024-12-27)


### Bug Fixes

* add new way to find main. release :bug: ([d4ccbe3](https://github.com/LerianStudio/midaz/commit/d4ccbe3ef80517eeb490466861b8960d522c228e))
* change command :bug: ([e7ef312](https://github.com/LerianStudio/midaz/commit/e7ef312e7eb5a2005a232d88f468258b45d1aab1))
* change command :bug: ([e55db05](https://github.com/LerianStudio/midaz/commit/e55db05ed0093409b41862a62b6a4197ef056994))
* change command :bug: ([558c982](https://github.com/LerianStudio/midaz/commit/558c9821785b36b36592af9b2395c06d4899dabe))
* change the way of trigger actions only to test :bug: ([a2236fc](https://github.com/LerianStudio/midaz/commit/a2236fc0c31c868afb31e4c5663ea62129ea2425))
* echo CURRENT_BRANCH :bug: ([27fd429](https://github.com/LerianStudio/midaz/commit/27fd429228c39a0d58285ec719e2e049ff5e5802))
* final :bug: ([831c6be](https://github.com/LerianStudio/midaz/commit/831c6befc64d0057a155a95bb717222b13346b0c))
* gh command :bug: ([8a76a21](https://github.com/LerianStudio/midaz/commit/8a76a21a1377ccc1d214831f18bcb63d3c6d5eba))
* print pr number and branch to merge pr :bug: ([a87b03f](https://github.com/LerianStudio/midaz/commit/a87b03fa3f69d88e8f6b1383e0e6cd38954322b0))
* remove echo :bug: ([abfdb69](https://github.com/LerianStudio/midaz/commit/abfdb6994ee625ec0ba84ac29d793e0576ced316))
* remove echos :bug: ([b329b11](https://github.com/LerianStudio/midaz/commit/b329b11c1e3231fc9f64b81c77e028f45d72235e))
* return right way trigger :bug: ([52bd136](https://github.com/LerianStudio/midaz/commit/52bd1368f2ca176bf3a906b4270b0e74e8112e9d))
* some echos :bug: ([12cb38d](https://github.com/LerianStudio/midaz/commit/12cb38dca1d3a0136d14ddb150856b38088e69ef))
* tag to print the real branch :bug: ([aa34991](https://github.com/LerianStudio/midaz/commit/aa349914022d3768fbfe25b3e0cebb441ba8f9f2))
* trigger :bug: ([9b62691](https://github.com/LerianStudio/midaz/commit/9b62691ea182703c53fb2dd1429332300e0ab4a5))
* unix adjusts :bug: ([df55e06](https://github.com/LerianStudio/midaz/commit/df55e061420c553359b84c41cc2b5b96d1cf3a3f))
* update :bug: ([5c91fbd](https://github.com/LerianStudio/midaz/commit/5c91fbdfe0560222ff0b62536f65656fad737d4c))
* update command :bug: ([3a3d470](https://github.com/LerianStudio/midaz/commit/3a3d47086b642b87b6df30945d53af62662bd061))
* update command :bug: ([faf8d8f](https://github.com/LerianStudio/midaz/commit/faf8d8f47802ee18163500a63b03371933e7b2a1))

## [1.40.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.40.0-beta.1...v1.40.0-beta.2) (2024-12-27)


### Bug Fixes

* add new way to find main. release :bug: ([d4ccbe3](https://github.com/LerianStudio/midaz/commit/d4ccbe3ef80517eeb490466861b8960d522c228e))
* change command :bug: ([e7ef312](https://github.com/LerianStudio/midaz/commit/e7ef312e7eb5a2005a232d88f468258b45d1aab1))
* change command :bug: ([e55db05](https://github.com/LerianStudio/midaz/commit/e55db05ed0093409b41862a62b6a4197ef056994))
* change command :bug: ([558c982](https://github.com/LerianStudio/midaz/commit/558c9821785b36b36592af9b2395c06d4899dabe))
* final :bug: ([831c6be](https://github.com/LerianStudio/midaz/commit/831c6befc64d0057a155a95bb717222b13346b0c))
* gh command :bug: ([8a76a21](https://github.com/LerianStudio/midaz/commit/8a76a21a1377ccc1d214831f18bcb63d3c6d5eba))
* print pr number and branch to merge pr :bug: ([a87b03f](https://github.com/LerianStudio/midaz/commit/a87b03fa3f69d88e8f6b1383e0e6cd38954322b0))
* remove echo :bug: ([abfdb69](https://github.com/LerianStudio/midaz/commit/abfdb6994ee625ec0ba84ac29d793e0576ced316))
* remove echos :bug: ([b329b11](https://github.com/LerianStudio/midaz/commit/b329b11c1e3231fc9f64b81c77e028f45d72235e))
* some echos :bug: ([12cb38d](https://github.com/LerianStudio/midaz/commit/12cb38dca1d3a0136d14ddb150856b38088e69ef))
* trigger :bug: ([9b62691](https://github.com/LerianStudio/midaz/commit/9b62691ea182703c53fb2dd1429332300e0ab4a5))
* unix adjusts :bug: ([df55e06](https://github.com/LerianStudio/midaz/commit/df55e061420c553359b84c41cc2b5b96d1cf3a3f))
* update :bug: ([5c91fbd](https://github.com/LerianStudio/midaz/commit/5c91fbdfe0560222ff0b62536f65656fad737d4c))
* update command :bug: ([3a3d470](https://github.com/LerianStudio/midaz/commit/3a3d47086b642b87b6df30945d53af62662bd061))
* update command :bug: ([faf8d8f](https://github.com/LerianStudio/midaz/commit/faf8d8f47802ee18163500a63b03371933e7b2a1))

## [1.40.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.39.0...v1.40.0-beta.1) (2024-12-27)


### Bug Fixes

* change the way of trigger actions only to test :bug: ([a2236fc](https://github.com/LerianStudio/midaz/commit/a2236fc0c31c868afb31e4c5663ea62129ea2425))
* echo CURRENT_BRANCH :bug: ([27fd429](https://github.com/LerianStudio/midaz/commit/27fd429228c39a0d58285ec719e2e049ff5e5802))
* return right way trigger :bug: ([52bd136](https://github.com/LerianStudio/midaz/commit/52bd1368f2ca176bf3a906b4270b0e74e8112e9d))
* tag to print the real branch :bug: ([aa34991](https://github.com/LerianStudio/midaz/commit/aa349914022d3768fbfe25b3e0cebb441ba8f9f2))

## [1.39.0](https://github.com/LerianStudio/midaz/compare/v1.38.1...v1.39.0) (2024-12-27)


### Features

* add tests to to print :sparkles: ([49bd3ce](https://github.com/LerianStudio/midaz/commit/49bd3ce6bbbe6a38a14a6378500b5a88367a2f7f))
* print release ([c915b67](https://github.com/LerianStudio/midaz/commit/c915b677409ff0564b2b75ce416d4b38243c81c2))
* update Choco install.ps1 with the latest release version :sparkles: ([3fbc073](https://github.com/LerianStudio/midaz/commit/3fbc0733d73970ca49a3808cb5ca85ae9e08f394))


### Bug Fixes

* add env test :bug: ([23351ae](https://github.com/LerianStudio/midaz/commit/23351aefa10786e7c9a6bea27b02ae5e5da5bc5d))
* add git token :bug: ([a42bf8b](https://github.com/LerianStudio/midaz/commit/a42bf8b0eaaac0c746b665e0fa6ed4979b25fa9e))
* add id to steps :bug: ([e6b22f8](https://github.com/LerianStudio/midaz/commit/e6b22f85bc43daa98e539e9c71b5329788c403df))
* add jobs with conditional :bug: ([fecc256](https://github.com/LerianStudio/midaz/commit/fecc256917173e6d4e3c0f7a35d2f5a49f342761))
* add more echo :bug: ([8d71809](https://github.com/LerianStudio/midaz/commit/8d7180977a5d23d74e2d52a1f1c12083ee89adc8))
* add new echos :bug: ([1d5f58d](https://github.com/LerianStudio/midaz/commit/1d5f58d83a093465f69735f5bfd3851a4be6cf8e))
* add on outputs :bug: ([967a3e6](https://github.com/LerianStudio/midaz/commit/967a3e6f45f61afc4df273fd2f46ec83f3270d32))
* add prints :bug: ([27e7c43](https://github.com/LerianStudio/midaz/commit/27e7c43567de2a6eb5aa44fb47571f8e2f6bd719))
* add same behavior in metadata transaction :bug: ([783ae69](https://github.com/LerianStudio/midaz/commit/783ae69fbcbb32ce1c46a363c532988eb0fb03e4))
* add steps if main or not :bug: ([dc7ac2e](https://github.com/LerianStudio/midaz/commit/dc7ac2e34af068c5a8df829bbe11e454d444edf1))
* add variables :bug: ([d69afd4](https://github.com/LerianStudio/midaz/commit/d69afd450765c42c98e73a560e7f0dfe4d180eb7))
* added print install.ps1 shell ([389c26a](https://github.com/LerianStudio/midaz/commit/389c26ae30e25876f8672f97f1e828c96c1e3f9a))
* added push choco ([ecd2580](https://github.com/LerianStudio/midaz/commit/ecd25809e3f2de081514de5d12614cc0335a29f7))
* added url hardcoded to test run package choco ([a6c9632](https://github.com/LerianStudio/midaz/commit/a6c963223943cf49cc096a3fecc7c298c50866ed))
* adjust ref variable :bug: ([f7bb3d8](https://github.com/LerianStudio/midaz/commit/f7bb3d8c59410001cd0797a51552006ade264209))
* adjust trigger git actions :bug: ([7dca6a7](https://github.com/LerianStudio/midaz/commit/7dca6a723972805a2392a237ca365426aeb8bcd6))
* check bin access cli mdz :bug: ([f93da6c](https://github.com/LerianStudio/midaz/commit/f93da6c1e7a6decb03e94c8e6089d0d583d13666))
* check var version value :bug: ([42731cc](https://github.com/LerianStudio/midaz/commit/42731cc894e3f78a7c0cb4ec4c0fe0e7a30fed16))
* choco pack added ([0d8876e](https://github.com/LerianStudio/midaz/commit/0d8876e82f2ffda20b2f6d8c63640675683a47f5))
* comment if check main to action choco to validated ([c55a9e8](https://github.com/LerianStudio/midaz/commit/c55a9e82e2472a5076654b7fa88257b217522a66))
* define shell before script ([3091d0d](https://github.com/LerianStudio/midaz/commit/3091d0d012c055a6fb3743282ed0269b71575241))
* final :bug: ([5c3347c](https://github.com/LerianStudio/midaz/commit/5c3347ce4b97631399d08ff89c5f58dba6ae8065))
* get branch and set to env :bug: ([4a5cde0](https://github.com/LerianStudio/midaz/commit/4a5cde0550df1d2a2ce7871625fba542612059cf))
* list itens ([f8da540](https://github.com/LerianStudio/midaz/commit/f8da540a9f64d33e22a29094cf2a143fff9ecb74))
* print job variable :bug: ([db155b3](https://github.com/LerianStudio/midaz/commit/db155b3718133acc483c6abee3ed8a426dfa7de1))
* print var env ([d3422e0](https://github.com/LerianStudio/midaz/commit/d3422e0a42d250ff4d2e80255ab17368c25be230))
* print var env choco ([a9e7be7](https://github.com/LerianStudio/midaz/commit/a9e7be78aeb4a6bf067ec7a2854999ca77f287db))
* push package in chocoloty :bug: ([4c17e0a](https://github.com/LerianStudio/midaz/commit/4c17e0ad1c1217a465d2884ddad8a5db24aadef9))
* remove query params url icon github ([bfcdbe3](https://github.com/LerianStudio/midaz/commit/bfcdbe320df061f0235cb9c0d8944f31371bac23))
* remove uuid wrong version and invalid variant from tests :bug: ([cf7587d](https://github.com/LerianStudio/midaz/commit/cf7587d0d34c1f3061d6f0acc93a5b7778e6415f))
* runner action package to commit ([3395dfa](https://github.com/LerianStudio/midaz/commit/3395dfa405bbc909ac447478eb02fd6bf4265fd3))
* tagname nuspec close errored ([f97e2a6](https://github.com/LerianStudio/midaz/commit/f97e2a62ab0a72c37edc41c2884e0cd952f6f763))
* try install local 2 testing :bug: ([7d1998f](https://github.com/LerianStudio/midaz/commit/7d1998f77dcb450b3172db7a4fcddf2264b70352))
* try install local testing :bug: ([2737ded](https://github.com/LerianStudio/midaz/commit/2737ded50d77704398661735009bc8adfa104777))
* update dispached action chocolaty in momment correct ([99cd335](https://github.com/LerianStudio/midaz/commit/99cd33503f2c8389445f186ef5f8b801da8ff2aa))
* update from and to :bug: ([dda0b79](https://github.com/LerianStudio/midaz/commit/dda0b79ed9975523c49fa853c0ffe0dde3f631b9))
* update func that check is uuid our alias from account :bug: ([f336c64](https://github.com/LerianStudio/midaz/commit/f336c64214345093e2e4d24162e206c2f8c975d3))
* update some adjusts :bug: ([61db1cf](https://github.com/LerianStudio/midaz/commit/61db1cfd6e90bee2e5649918481c7d4361ac00a1))
* update version final :bug: ([668ede4](https://github.com/LerianStudio/midaz/commit/668ede4c692d541c8d91df1a47335db06e557ac5))
* used var version correct ([cf11623](https://github.com/LerianStudio/midaz/commit/cf11623cf73e0bef14121d45f2c9d0fafc905f13))

## [1.39.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.39.0-beta.2...v1.39.0-beta.3) (2024-12-27)


### Features

* add tests to to print :sparkles: ([49bd3ce](https://github.com/LerianStudio/midaz/commit/49bd3ce6bbbe6a38a14a6378500b5a88367a2f7f))
* print release ([c915b67](https://github.com/LerianStudio/midaz/commit/c915b677409ff0564b2b75ce416d4b38243c81c2))
* update Choco install.ps1 with the latest release version :sparkles: ([3fbc073](https://github.com/LerianStudio/midaz/commit/3fbc0733d73970ca49a3808cb5ca85ae9e08f394))


### Bug Fixes

* add env test :bug: ([23351ae](https://github.com/LerianStudio/midaz/commit/23351aefa10786e7c9a6bea27b02ae5e5da5bc5d))
* add git token :bug: ([a42bf8b](https://github.com/LerianStudio/midaz/commit/a42bf8b0eaaac0c746b665e0fa6ed4979b25fa9e))
* add id to steps :bug: ([e6b22f8](https://github.com/LerianStudio/midaz/commit/e6b22f85bc43daa98e539e9c71b5329788c403df))
* add jobs with conditional :bug: ([fecc256](https://github.com/LerianStudio/midaz/commit/fecc256917173e6d4e3c0f7a35d2f5a49f342761))
* add more echo :bug: ([8d71809](https://github.com/LerianStudio/midaz/commit/8d7180977a5d23d74e2d52a1f1c12083ee89adc8))
* add new echos :bug: ([1d5f58d](https://github.com/LerianStudio/midaz/commit/1d5f58d83a093465f69735f5bfd3851a4be6cf8e))
* add on outputs :bug: ([967a3e6](https://github.com/LerianStudio/midaz/commit/967a3e6f45f61afc4df273fd2f46ec83f3270d32))
* add prints :bug: ([27e7c43](https://github.com/LerianStudio/midaz/commit/27e7c43567de2a6eb5aa44fb47571f8e2f6bd719))
* add steps if main or not :bug: ([dc7ac2e](https://github.com/LerianStudio/midaz/commit/dc7ac2e34af068c5a8df829bbe11e454d444edf1))
* add variables :bug: ([d69afd4](https://github.com/LerianStudio/midaz/commit/d69afd450765c42c98e73a560e7f0dfe4d180eb7))
* added print install.ps1 shell ([389c26a](https://github.com/LerianStudio/midaz/commit/389c26ae30e25876f8672f97f1e828c96c1e3f9a))
* added push choco ([ecd2580](https://github.com/LerianStudio/midaz/commit/ecd25809e3f2de081514de5d12614cc0335a29f7))
* added url hardcoded to test run package choco ([a6c9632](https://github.com/LerianStudio/midaz/commit/a6c963223943cf49cc096a3fecc7c298c50866ed))
* adjust ref variable :bug: ([f7bb3d8](https://github.com/LerianStudio/midaz/commit/f7bb3d8c59410001cd0797a51552006ade264209))
* check bin access cli mdz :bug: ([f93da6c](https://github.com/LerianStudio/midaz/commit/f93da6c1e7a6decb03e94c8e6089d0d583d13666))
* check var version value :bug: ([42731cc](https://github.com/LerianStudio/midaz/commit/42731cc894e3f78a7c0cb4ec4c0fe0e7a30fed16))
* choco pack added ([0d8876e](https://github.com/LerianStudio/midaz/commit/0d8876e82f2ffda20b2f6d8c63640675683a47f5))
* comment if check main to action choco to validated ([c55a9e8](https://github.com/LerianStudio/midaz/commit/c55a9e82e2472a5076654b7fa88257b217522a66))
* define shell before script ([3091d0d](https://github.com/LerianStudio/midaz/commit/3091d0d012c055a6fb3743282ed0269b71575241))
* final :bug: ([5c3347c](https://github.com/LerianStudio/midaz/commit/5c3347ce4b97631399d08ff89c5f58dba6ae8065))
* get branch and set to env :bug: ([4a5cde0](https://github.com/LerianStudio/midaz/commit/4a5cde0550df1d2a2ce7871625fba542612059cf))
* list itens ([f8da540](https://github.com/LerianStudio/midaz/commit/f8da540a9f64d33e22a29094cf2a143fff9ecb74))
* print job variable :bug: ([db155b3](https://github.com/LerianStudio/midaz/commit/db155b3718133acc483c6abee3ed8a426dfa7de1))
* print var env ([d3422e0](https://github.com/LerianStudio/midaz/commit/d3422e0a42d250ff4d2e80255ab17368c25be230))
* print var env choco ([a9e7be7](https://github.com/LerianStudio/midaz/commit/a9e7be78aeb4a6bf067ec7a2854999ca77f287db))
* push package in chocoloty :bug: ([4c17e0a](https://github.com/LerianStudio/midaz/commit/4c17e0ad1c1217a465d2884ddad8a5db24aadef9))
* remove query params url icon github ([bfcdbe3](https://github.com/LerianStudio/midaz/commit/bfcdbe320df061f0235cb9c0d8944f31371bac23))
* runner action package to commit ([3395dfa](https://github.com/LerianStudio/midaz/commit/3395dfa405bbc909ac447478eb02fd6bf4265fd3))
* tagname nuspec close errored ([f97e2a6](https://github.com/LerianStudio/midaz/commit/f97e2a62ab0a72c37edc41c2884e0cd952f6f763))
* try install local 2 testing :bug: ([7d1998f](https://github.com/LerianStudio/midaz/commit/7d1998f77dcb450b3172db7a4fcddf2264b70352))
* try install local testing :bug: ([2737ded](https://github.com/LerianStudio/midaz/commit/2737ded50d77704398661735009bc8adfa104777))
* update some adjusts :bug: ([61db1cf](https://github.com/LerianStudio/midaz/commit/61db1cfd6e90bee2e5649918481c7d4361ac00a1))
* update version final :bug: ([668ede4](https://github.com/LerianStudio/midaz/commit/668ede4c692d541c8d91df1a47335db06e557ac5))
* used var version correct ([cf11623](https://github.com/LerianStudio/midaz/commit/cf11623cf73e0bef14121d45f2c9d0fafc905f13))

## [1.39.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.39.0-beta.1...v1.39.0-beta.2) (2024-12-26)


### Bug Fixes

* add same behavior in metadata transaction :bug: ([783ae69](https://github.com/LerianStudio/midaz/commit/783ae69fbcbb32ce1c46a363c532988eb0fb03e4))
* adjust trigger git actions :bug: ([7dca6a7](https://github.com/LerianStudio/midaz/commit/7dca6a723972805a2392a237ca365426aeb8bcd6))
* remove uuid wrong version and invalid variant from tests :bug: ([cf7587d](https://github.com/LerianStudio/midaz/commit/cf7587d0d34c1f3061d6f0acc93a5b7778e6415f))
* update from and to :bug: ([dda0b79](https://github.com/LerianStudio/midaz/commit/dda0b79ed9975523c49fa853c0ffe0dde3f631b9))
* update func that check is uuid our alias from account :bug: ([f336c64](https://github.com/LerianStudio/midaz/commit/f336c64214345093e2e4d24162e206c2f8c975d3))

## [1.39.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.38.1...v1.39.0-beta.1) (2024-12-26)


### Bug Fixes

* update dispached action chocolaty in momment correct ([99cd335](https://github.com/LerianStudio/midaz/commit/99cd33503f2c8389445f186ef5f8b801da8ff2aa))

## [1.38.1](https://github.com/LerianStudio/midaz/compare/v1.38.0...v1.38.1) (2024-12-26)

## [1.38.1-beta.1](https://github.com/LerianStudio/midaz/compare/v1.38.0...v1.38.1-beta.1) (2024-12-26)

## [1.38.0](https://github.com/LerianStudio/midaz/compare/v1.37.0...v1.38.0) (2024-12-23)


### Bug Fixes

* add release verification to ensure valid conditions for brew and choco jobs ([3e2f473](https://github.com/LerianStudio/midaz/commit/3e2f473baf9e9f7df6ae5a38c55b9cd2d0dd8e6c))
* syntax error in release script (missing 'fi') ([da2c053](https://github.com/LerianStudio/midaz/commit/da2c0532b1d9fbb1fcbcf57b5953f98981adcb7f))

## [1.38.0-beta.5](https://github.com/LerianStudio/midaz/compare/v1.38.0-beta.4...v1.38.0-beta.5) (2024-12-23)

## [1.38.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.38.0-beta.3...v1.38.0-beta.4) (2024-12-23)

## [1.38.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.38.0-beta.2...v1.38.0-beta.3) (2024-12-23)

## [1.38.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.38.0-beta.1...v1.38.0-beta.2) (2024-12-23)


### Bug Fixes

* syntax error in release script (missing 'fi') ([da2c053](https://github.com/LerianStudio/midaz/commit/da2c0532b1d9fbb1fcbcf57b5953f98981adcb7f))

## [1.38.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.37.0...v1.38.0-beta.1) (2024-12-23)


### Bug Fixes

* add release verification to ensure valid conditions for brew and choco jobs ([3e2f473](https://github.com/LerianStudio/midaz/commit/3e2f473baf9e9f7df6ae5a38c55b9cd2d0dd8e6c))

## [1.37.0](https://github.com/LerianStudio/midaz/compare/v1.36.0...v1.37.0) (2024-12-23)


### Features

* publish cli midaz in the choco ([fac503f](https://github.com/LerianStudio/midaz/commit/fac503f593fe344ba47e3aebc9a4e880c5661d68))


### Bug Fixes

* help :bug: ([8f9358f](https://github.com/LerianStudio/midaz/commit/8f9358f0d0608ce82f96084f0761de6edc7dcdae))
* **audit:** improve makefile changing docker-compose to a choose based version command :bug: ([14506fe](https://github.com/LerianStudio/midaz/commit/14506fec46ee5f5c946d67c7fc63135a08a739aa))
* **auth:** improve makefile changing docker-compose to a choose based version command :bug: ([d9ee74c](https://github.com/LerianStudio/midaz/commit/d9ee74c64e3e64e0b06ac390dd0feda4810b4daf))
* **infra:** improve makefile changing docker-compose to a choose based version command :bug: ([e43892c](https://github.com/LerianStudio/midaz/commit/e43892ce9eb930b2f57e0814b0b3793be11c8be7))
* **ledger:** improve makefile changing docker-compose to a choose based version command :bug: ([89552e0](https://github.com/LerianStudio/midaz/commit/89552e066cbf45e18d8d824d9ad025ef48bee71b))
* **midaz:** improve makefile changing docker-compose to a choose based version command :bug: ([9951e8c](https://github.com/LerianStudio/midaz/commit/9951e8c706c12bcee8fadf6af01186a82834f547))
* **transaction:** improve makefile changing docker-compose to a choose based version command :bug: ([44a1b1f](https://github.com/LerianStudio/midaz/commit/44a1b1fc977b85bedb59a368075089f0b2d5da2c))
* info :bug: ([3f01ba4](https://github.com/LerianStudio/midaz/commit/3f01ba4452082a728580a0296f4b64bff6c40e16))
* remove wire reference :bug: ([a7c61ee](https://github.com/LerianStudio/midaz/commit/a7c61ee2426ec1523d7750fc22041e9478f9ebad))

## [1.37.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.37.0-beta.1...v1.37.0-beta.2) (2024-12-20)


### Features

* publish cli midaz in the choco ([fac503f](https://github.com/LerianStudio/midaz/commit/fac503f593fe344ba47e3aebc9a4e880c5661d68))

## [1.37.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.36.0...v1.37.0-beta.1) (2024-12-20)


### Bug Fixes

* help :bug: ([8f9358f](https://github.com/LerianStudio/midaz/commit/8f9358f0d0608ce82f96084f0761de6edc7dcdae))
* **audit:** improve makefile changing docker-compose to a choose based version command :bug: ([14506fe](https://github.com/LerianStudio/midaz/commit/14506fec46ee5f5c946d67c7fc63135a08a739aa))
* **auth:** improve makefile changing docker-compose to a choose based version command :bug: ([d9ee74c](https://github.com/LerianStudio/midaz/commit/d9ee74c64e3e64e0b06ac390dd0feda4810b4daf))
* **infra:** improve makefile changing docker-compose to a choose based version command :bug: ([e43892c](https://github.com/LerianStudio/midaz/commit/e43892ce9eb930b2f57e0814b0b3793be11c8be7))
* **ledger:** improve makefile changing docker-compose to a choose based version command :bug: ([89552e0](https://github.com/LerianStudio/midaz/commit/89552e066cbf45e18d8d824d9ad025ef48bee71b))
* **midaz:** improve makefile changing docker-compose to a choose based version command :bug: ([9951e8c](https://github.com/LerianStudio/midaz/commit/9951e8c706c12bcee8fadf6af01186a82834f547))
* **transaction:** improve makefile changing docker-compose to a choose based version command :bug: ([44a1b1f](https://github.com/LerianStudio/midaz/commit/44a1b1fc977b85bedb59a368075089f0b2d5da2c))
* info :bug: ([3f01ba4](https://github.com/LerianStudio/midaz/commit/3f01ba4452082a728580a0296f4b64bff6c40e16))
* remove wire reference :bug: ([a7c61ee](https://github.com/LerianStudio/midaz/commit/a7c61ee2426ec1523d7750fc22041e9478f9ebad))

## [1.36.0](https://github.com/LerianStudio/midaz/compare/v1.35.0...v1.36.0) (2024-12-19)


### Features

* add new oas 3.0 yaml ([d34059d](https://github.com/LerianStudio/midaz/commit/d34059d2e205f1a7c45ae55e2dc2bf56d6cba056))


### Bug Fixes

* remane to correct spelling name :bug: ([79da5bc](https://github.com/LerianStudio/midaz/commit/79da5bc9118edf793ae1c179d0dde8a04fe3e2e8))
* remove docker compose swag generate :bug: ([a65b347](https://github.com/LerianStudio/midaz/commit/a65b34773772b55ecfc19607570548f1182d1acd))

## [1.36.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.35.0...v1.36.0-beta.1) (2024-12-19)


### Features

* add new oas 3.0 yaml ([d34059d](https://github.com/LerianStudio/midaz/commit/d34059d2e205f1a7c45ae55e2dc2bf56d6cba056))


### Bug Fixes

* remane to correct spelling name :bug: ([79da5bc](https://github.com/LerianStudio/midaz/commit/79da5bc9118edf793ae1c179d0dde8a04fe3e2e8))
* remove docker compose swag generate :bug: ([a65b347](https://github.com/LerianStudio/midaz/commit/a65b34773772b55ecfc19607570548f1182d1acd))

## [1.35.0](https://github.com/LerianStudio/midaz/compare/v1.34.0...v1.35.0) (2024-12-19)


### Bug Fixes

* publish pr homebrew after release publish ([ff10d3a](https://github.com/LerianStudio/midaz/commit/ff10d3ae5feba75149bbdcbe882048a3e248e44d))

## [1.35.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.34.0...v1.35.0-beta.1) (2024-12-19)


### Bug Fixes

* publish pr homebrew after release publish ([ff10d3a](https://github.com/LerianStudio/midaz/commit/ff10d3ae5feba75149bbdcbe882048a3e248e44d))

## [1.34.0](https://github.com/LerianStudio/midaz/compare/v1.33.0...v1.34.0) (2024-12-19)


### Features

* add http utils func that help to get idempotency key and ttl ([cce1bcf](https://github.com/LerianStudio/midaz/commit/cce1bcfc8add465849cb1d36caad7a29ba883648))
* create a func that convert uuid.string to uuid :sparkles: ([f535250](https://github.com/LerianStudio/midaz/commit/f53525007586cb61bcee7a21406fca2036d062f2))
* create a string to sha-256 convert func :sparkles: ([c675219](https://github.com/LerianStudio/midaz/commit/c675219aeba97b08151e10c2f4e3a228cbed4094))
* create header key to Idempotency and ttl :sparkles: ([b9124ed](https://github.com/LerianStudio/midaz/commit/b9124ed2991f2caae8c04880bbcdfc7151f30e63))
* create Idempotency business error ([4c18470](https://github.com/LerianStudio/midaz/commit/4c18470c9ce56ae1e5d59f8ea1af55627539d1d8))


### Bug Fixes

* added if pre release bump formule brew ([0c576cc](https://github.com/LerianStudio/midaz/commit/0c576cc4bddbcafe3b5a3277e19a284e36ff4672))
* adjust redis consumer :bug: ([f7edc63](https://github.com/LerianStudio/midaz/commit/f7edc63b4a775bcc5fdb32ac57f5557dc973bad9))
* change ubuntu-release to ubuntu-24.04 :bug: ([8044333](https://github.com/LerianStudio/midaz/commit/804433391ea027c4275ce75ba967b473e151b5f7))
* go test :bug: ([ab60f3b](https://github.com/LerianStudio/midaz/commit/ab60f3bf4dc6ff58ad9ce4f2ceb417aa0e049512))
* golint :bug: ([32083c3](https://github.com/LerianStudio/midaz/commit/32083c3deb72794b43cb3928e03a5fe1a1ad8c19))
* remove if :bug: ([d6e370a](https://github.com/LerianStudio/midaz/commit/d6e370ad97473314e07f70f091401ffdcfd2f666))
* this is the correct pattern to match all branches that start with hotfix/ followed by any text. :bug: ([0ff571c](https://github.com/LerianStudio/midaz/commit/0ff571cdbe4a16dc8eaed805f27f1fee529a09fb))
* update git action :bug: ([7acb849](https://github.com/LerianStudio/midaz/commit/7acb8494480685e708d808fafe179b8b110ade6e))
* update go mod :bug: ([5ead58a](https://github.com/LerianStudio/midaz/commit/5ead58af6b04569123c6d9ef52128e1b962b3c05))

## [1.34.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.34.0-beta.1...v1.34.0-beta.2) (2024-12-19)


### Features

* add http utils func that help to get idempotency key and ttl ([cce1bcf](https://github.com/LerianStudio/midaz/commit/cce1bcfc8add465849cb1d36caad7a29ba883648))
* create a func that convert uuid.string to uuid :sparkles: ([f535250](https://github.com/LerianStudio/midaz/commit/f53525007586cb61bcee7a21406fca2036d062f2))
* create a string to sha-256 convert func :sparkles: ([c675219](https://github.com/LerianStudio/midaz/commit/c675219aeba97b08151e10c2f4e3a228cbed4094))
* create header key to Idempotency and ttl :sparkles: ([b9124ed](https://github.com/LerianStudio/midaz/commit/b9124ed2991f2caae8c04880bbcdfc7151f30e63))
* create Idempotency business error ([4c18470](https://github.com/LerianStudio/midaz/commit/4c18470c9ce56ae1e5d59f8ea1af55627539d1d8))


### Bug Fixes

* adjust redis consumer :bug: ([f7edc63](https://github.com/LerianStudio/midaz/commit/f7edc63b4a775bcc5fdb32ac57f5557dc973bad9))
* go test :bug: ([ab60f3b](https://github.com/LerianStudio/midaz/commit/ab60f3bf4dc6ff58ad9ce4f2ceb417aa0e049512))
* golint :bug: ([32083c3](https://github.com/LerianStudio/midaz/commit/32083c3deb72794b43cb3928e03a5fe1a1ad8c19))
* update go mod :bug: ([5ead58a](https://github.com/LerianStudio/midaz/commit/5ead58af6b04569123c6d9ef52128e1b962b3c05))

## [1.34.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.33.0...v1.34.0-beta.1) (2024-12-19)


### Bug Fixes

* added if pre release bump formule brew ([0c576cc](https://github.com/LerianStudio/midaz/commit/0c576cc4bddbcafe3b5a3277e19a284e36ff4672))
* change ubuntu-release to ubuntu-24.04 :bug: ([8044333](https://github.com/LerianStudio/midaz/commit/804433391ea027c4275ce75ba967b473e151b5f7))
* remove if :bug: ([d6e370a](https://github.com/LerianStudio/midaz/commit/d6e370ad97473314e07f70f091401ffdcfd2f666))
* this is the correct pattern to match all branches that start with hotfix/ followed by any text. :bug: ([0ff571c](https://github.com/LerianStudio/midaz/commit/0ff571cdbe4a16dc8eaed805f27f1fee529a09fb))
* update git action :bug: ([7acb849](https://github.com/LerianStudio/midaz/commit/7acb8494480685e708d808fafe179b8b110ade6e))

## [1.33.0](https://github.com/LerianStudio/midaz/compare/v1.32.0...v1.33.0) (2024-12-18)


### Bug Fixes

* if only in main base :bug: ([4cc3bbb](https://github.com/LerianStudio/midaz/commit/4cc3bbb97f2bf7c3c3c81ad43f91d871fdfd08b9))
* separated github actions from a different one file apart :bug: ([c72c00f](https://github.com/LerianStudio/midaz/commit/c72c00f2dbfd87286717438e6bb026a5bd3bb82b))

## [1.32.0](https://github.com/LerianStudio/midaz/compare/v1.31.0...v1.32.0) (2024-12-18)


### Bug Fixes

* Add 'workflow_run' for bump_formula and update dependencies ([7e67ac0](https://github.com/LerianStudio/midaz/commit/7e67ac0bfa4c39850c4b211ae9b6ed4adf7aba7b))

## [1.32.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.31.1-beta.2...v1.32.0-beta.1) (2024-12-18)


### Bug Fixes

* Add 'workflow_run' for bump_formula and update dependencies ([7e67ac0](https://github.com/LerianStudio/midaz/commit/7e67ac0bfa4c39850c4b211ae9b6ed4adf7aba7b))

## [1.31.1](https://github.com/LerianStudio/midaz/compare/v1.31.0...v1.31.1) (2024-12-18)

## [1.31.1-beta.2](https://github.com/LerianStudio/midaz/compare/v1.31.1-beta.1...v1.31.1-beta.2) (2024-12-18)

## [1.31.1-beta.1](https://github.com/LerianStudio/midaz/compare/v1.31.0...v1.31.1-beta.1) (2024-12-18)

## [1.31.0](https://github.com/LerianStudio/midaz/compare/v1.30.0...v1.31.0) (2024-12-17)


### Features

*  finish redis verbs :sparkles: ([5ae2ddc](https://github.com/LerianStudio/midaz/commit/5ae2ddc8437b9be4fdaf7f8113cf4bb082aa16df))
* **audit:** add audit logs handler :sparkles: ([4a5fe36](https://github.com/LerianStudio/midaz/commit/4a5fe36a69fb9342d962c07a5fafdb64bbdfcfa4))
* **audit:** add authorization for routes :sparkles: ([2700d50](https://github.com/LerianStudio/midaz/commit/2700d50d93fbdb2203dc8ef19335d26f86737e45))
* **asset-rate:** add cursor pagination to get all entities endpoint :sparkles: ([441c51c](https://github.com/LerianStudio/midaz/commit/441c51c5e5d27672f2c87cfbf3b512b85bf38798))
* **transaction:** add cursor pagination to get all entities endpoint :sparkles: ([9b1cb94](https://github.com/LerianStudio/midaz/commit/9b1cb9405e6dc86a27aeb1b8980d9a68ce430734))
* **operation:** add cursor pagination to get all entities endpoints :sparkles: ([21315a3](https://github.com/LerianStudio/midaz/commit/21315a317cf0ed329900769980fa5fc3fb7f17ce))
* **audit:** add custom error messages :sparkles: ([db9bc72](https://github.com/LerianStudio/midaz/commit/db9bc72195faa6bbb6d143260baa34e0db7d032c))
* **audit:** add get audit info use case :sparkles: ([9cc6503](https://github.com/LerianStudio/midaz/commit/9cc65035dd99edb6a1626acc67efec0d1fad108d))
* **audit:** add get log by hash use case :sparkles: ([66d3b93](https://github.com/LerianStudio/midaz/commit/66d3b9379ac47475f9f32f9fe70e1c52ce9d46b7))
* **audit:** add methods for retrieving trillian inclusion proof and leaf by index :sparkles: ([03b12bd](https://github.com/LerianStudio/midaz/commit/03b12bdd406cab91295d0bd21de96574f8c09e53))
* **postman:** add pagination fields to postman for get all endpoints :sparkles: ([63e3e56](https://github.com/LerianStudio/midaz/commit/63e3e56b033cbddc8edb01d986c5e37c6d060834))
* **pagination:** add sort order filter and date ranges to the midaz pagination filtering :sparkles: ([4cc01d3](https://github.com/LerianStudio/midaz/commit/4cc01d311f51c16b759d7e8e1e287193eafab0d8))
* **audit:** add trace spans :sparkles: ([1ea30fa](https://github.com/LerianStudio/midaz/commit/1ea30fab9d2c75bebd51309d709a9b833d0b66d4))
* **audit:** add trillian health check before connecting :sparkles: ([9295cec](https://github.com/LerianStudio/midaz/commit/9295cec1036dd77da7d843c38603247be2d46ed5))
* add update swagger audit on git pages ([137824a](https://github.com/LerianStudio/midaz/commit/137824a9f721e140a4ecb7ec08cca07c99762b59))
* **audit:** add validate log use case :sparkles: ([7216c5e](https://github.com/LerianStudio/midaz/commit/7216c5e744d0246961db040f6c045c60452b1dc1))
* added command configure :sparkles: ([f269cf3](https://github.com/LerianStudio/midaz/commit/f269cf3c6a9f3badd2cea2bf93982433ff72e4af))
* added new flags to get list filters :sparkles: ([959cc9d](https://github.com/LerianStudio/midaz/commit/959cc9db71a40b963af279be17e8be48aa79b123))
* async log transaction call :sparkles: ([35816e4](https://github.com/LerianStudio/midaz/commit/35816e444d153a4e555ab5708a20d3635ffe69da))
* audit component :sparkles: ([084603f](https://github.com/LerianStudio/midaz/commit/084603f08386b7ebcfa67eaac7b094ddf676976f))
* **audit:** audit structs to aux mongo database ([4b80b75](https://github.com/LerianStudio/midaz/commit/4b80b75a16cefb77a4908e04b7ac522e347fb8eb))
* check diff before commit changes ([4e5d2d3](https://github.com/LerianStudio/midaz/commit/4e5d2d3e3ac09cbd7819fdba7ba2eed24ff975ff))
* configure command created defines the envs variables used in ldflags via command with the unit test of the ending with print command and print fields :sparkles: ([f407ab8](https://github.com/LerianStudio/midaz/commit/f407ab85224d30aa9f923dd27f9f49e76669e3d4))
* copy swagger.josn and check diff ([1cd0658](https://github.com/LerianStudio/midaz/commit/1cd0658dacd9747d4bd08b6d3f5b1e742791d115))
* create audit app ([f3f8cd5](https://github.com/LerianStudio/midaz/commit/f3f8cd5f3e7e8023e17e1f17111e9e221ec62227))
* **auth:** create auditor user :sparkles: ([5953ad9](https://github.com/LerianStudio/midaz/commit/5953ad9ac44faa3c8c9014eb47d480176f6d49ca))
* create operation log struct to specify fields that should be immutable :sparkles: ([b5438c1](https://github.com/LerianStudio/midaz/commit/b5438c15eba68b1e35683a41507eb5105cafa140))
* create route consumer for many queues ([8004063](https://github.com/LerianStudio/midaz/commit/8004063186d6c85bd0ed99e5d081acdc9ecdfb8f))
* **audit:** create struct for queue messages :sparkles: ([646bd38](https://github.com/LerianStudio/midaz/commit/646bd38cced4fc57f51fb2e5bd3d3137ba2a83bc))
* **audit:** create structs for audit transaction message ([fa6b568](https://github.com/LerianStudio/midaz/commit/fa6b568d83b165d59540ee7878e550f81ddc3789))
* **audit:** create transaction logs from rabbitmq message ([d54e4d3](https://github.com/LerianStudio/midaz/commit/d54e4d387e08ea5d9a47898bd0e94df7ab5c2f5d))
* **audit:** create trillian log leaf ([d18c0c2](https://github.com/LerianStudio/midaz/commit/d18c0c22f540196e575bfc1f0656da1fb5747a54))
* disable audit logging thought env :sparkles: ([8fa77c8](https://github.com/LerianStudio/midaz/commit/8fa77c8871073c613c55f695722ffcf15240a17a))
* **audit:** errors return from log creation :sparkles: ([69594e4](https://github.com/LerianStudio/midaz/commit/69594e4e24eb6d107b2d8fa27f83d0e76e058405))
* **audit:** find audit info by ID :sparkles: ([ea91e97](https://github.com/LerianStudio/midaz/commit/ea91e971ac2db8cd8a7befe2e42d994e6987902f))
* generate swagger on midaz ([3678070](https://github.com/LerianStudio/midaz/commit/3678070fbf0f105359ec0206aed8cbacd26f5e06))
* get audit exchange and routing key names from envs :sparkles: ([ce70e91](https://github.com/LerianStudio/midaz/commit/ce70e9106c715a18225c4cf50f234b891de46dc0))
* **audit:** ignore updatable fields for operation :sparkles: ([28db38d](https://github.com/LerianStudio/midaz/commit/28db38d0e391904458fd8234303f6f56b412e6c3))
* **audit:** implement get trillian log by hash :sparkles: ([44d103b](https://github.com/LerianStudio/midaz/commit/44d103bbef1e80acd37ecd5c5e3d4ce238ea8530))
* **audit:** implement rabbitmq consumer :sparkles: ([9874dc4](https://github.com/LerianStudio/midaz/commit/9874dc453cfcbb94379c9e256c6aeeacef136bc9))
* implement trillian connection :sparkles: ([c4b8877](https://github.com/LerianStudio/midaz/commit/c4b887706dd4ce4ea8c7f7358ff40854f60bc2a6))
* **audit:** implements read logs by transaction handler :sparkles: ([d134b07](https://github.com/LerianStudio/midaz/commit/d134b07e7715f05b9e32817a47fe95ded1721c7b))
* **pagination:** improve pagination validations and tests :sparkles: ([8226e87](https://github.com/LerianStudio/midaz/commit/8226e87338c1a847c85301cd3752420e2b8cb1a7))
* **audit:** receiving audit parameter to create tree ([be43f32](https://github.com/LerianStudio/midaz/commit/be43f324ac21354c60a65ce2beda5f1c4f78871f))
* remove correlation-id after midaz-id implemented :sparkles: ([63e8016](https://github.com/LerianStudio/midaz/commit/63e80169edfabba82848b61d56486866b8763c1f))
* **ledger:** remove exchange and key from connection :sparkles: ([621cbf9](https://github.com/LerianStudio/midaz/commit/621cbf949446f216aae08f5b9bead44afb90c01e))
* remove exchange and key from rabbitmq connection config :sparkles: ([aa086a1](https://github.com/LerianStudio/midaz/commit/aa086a160badcb6f57f589ffc2a5315db2a35e13))
* **audit:** returning log leaf instead of the value :sparkles: ([9b40d88](https://github.com/LerianStudio/midaz/commit/9b40d88189c3021e637cc3ce52686895b5b83130))
* right way of starter audit with only one queue consumer ([15a0a8c](https://github.com/LerianStudio/midaz/commit/15a0a8c9438d31597d749d3180adcd4a9eb994bc))
* send log message after transaction created :sparkles: ([66f3f64](https://github.com/LerianStudio/midaz/commit/66f3f64065654f1bcc292e458edb667a2296b5e5))
* soft delete asset and its external account :sparkles: ([7b090ba](https://github.com/LerianStudio/midaz/commit/7b090baf368be777a23c26e09e2ee33a0bbc4e91))
* **audit:** starting implementation of server :sparkles: ([edbce7b](https://github.com/LerianStudio/midaz/commit/edbce7bc2281c7d1273215dc372573e58680119c))
* steps to send slack message with release ([8957369](https://github.com/LerianStudio/midaz/commit/89573696f68c0a0ab20013cd265ea09874f02da5))
* test by specific branch ([a0f7af3](https://github.com/LerianStudio/midaz/commit/a0f7af3613d42ef23bf9f5f250a1fe7e58c7155a))
* **audit:** update audit info collection name :sparkles: ([7cd39fa](https://github.com/LerianStudio/midaz/commit/7cd39fa0861b06c7a728f9c25e44f656d2be7b50))
* **audit:** update audit route paths :sparkles: ([0f12899](https://github.com/LerianStudio/midaz/commit/0f128998b6525c4419e3e4acd388aac97e92cb48))
* update git pages ([1c6f8cc](https://github.com/LerianStudio/midaz/commit/1c6f8ccb098563a8ad2940a192cdcc6903ed686a))
* update pages with each json swagger ([b4d8563](https://github.com/LerianStudio/midaz/commit/b4d856369d400a829a0510ae02801c8f69d62b4b))
* update producer to receive Queue message, exchange and key through parameters :sparkles: ([8dc41f3](https://github.com/LerianStudio/midaz/commit/8dc41f3f94935297506ff12507626329ea52d669))
* **ledger:** update rabbitmq producer :sparkles: ([47e3eef](https://github.com/LerianStudio/midaz/commit/47e3eef87a62b87fdc61e9564e6e5bc5c7f9da2a))
* update swagger to teste commit ([b6aa4bf](https://github.com/LerianStudio/midaz/commit/b6aa4bfcd42a06cac72ccb7f3ab766024ea23315))
* **tests:** update tests with to pagination filter struct :sparkles: ([793b685](https://github.com/LerianStudio/midaz/commit/793b685541ebcb5c3897d585380f16d2f9705d37))
* **audit:** using generic queue struct instead of transaction to write logs :sparkles: ([4c1b86f](https://github.com/LerianStudio/midaz/commit/4c1b86f0f374d182ee39b430ea19b641bad4eca0))
* utils to convert string into hash to use on redis using idempotency :sparkles: ([9a64020](https://github.com/LerianStudio/midaz/commit/9a64020ea3da73eec9d7b7773cf12a7f2ea2e1ce))
* valida if has changes ([ac7cbdb](https://github.com/LerianStudio/midaz/commit/ac7cbdbc2bb621c9ff8c38bb4f407a86279c0f96))
* **audit:** work with generic audit log values :sparkles: ([9beb218](https://github.com/LerianStudio/midaz/commit/9beb21876f2cc57aacaabee502c45712e68102db))


### Bug Fixes

* **audit:** add audit_id parameter to uuid path parameters constant :bug: ([dcbcb05](https://github.com/LerianStudio/midaz/commit/dcbcb05de4d2f1cfb9340a807f299af6bb302c5f))
* **account:** add error message translation for prohibited external account creation and adjust validation assertion :bug: ([fdd5971](https://github.com/LerianStudio/midaz/commit/fdd59717c8cc8e419817ddea145a91ef7601d35a))
* add get git token to get tag version :bug: ([92b91e6](https://github.com/LerianStudio/midaz/commit/92b91e6c9306568e7a48a95311e82ef8a2ce2463))
* add more actions with same background :bug: ([cdd8164](https://github.com/LerianStudio/midaz/commit/cdd8164c08f51e1d421eb00f67f46077ffcd35e4))
* add more rules and shrink actions :bug: ([ce2b916](https://github.com/LerianStudio/midaz/commit/ce2b916599073f9baea9c11d2860b2c77c712523))
* add slash to the forbidden account external aliases :bug: ([5e28fd5](https://github.com/LerianStudio/midaz/commit/5e28fd56fa2a61a2566a07690db97c01163561f3))
* **audit:** add tree size validation to fix vulnerability :bug: ([313dbf4](https://github.com/LerianStudio/midaz/commit/313dbf40f06d088e2d36282f57a7585db3e5ab7a))
* add validation to patch and delete methods for external accounts on ledger :bug: ([96ba359](https://github.com/LerianStudio/midaz/commit/96ba359993badc9456ea9d9de9286e33a9b051aa))
* adjust filter by metadata on get all transactions endpoint :bug: ([18c93a7](https://github.com/LerianStudio/midaz/commit/18c93a77b59d4e5d34d50d293534eebae3e22f60))
* adjust path :bug: ([41ec839](https://github.com/LerianStudio/midaz/commit/41ec839fc9a792229503f036b4e6e267cb8010cd))
* adjust to change run :bug: ([bad23fe](https://github.com/LerianStudio/midaz/commit/bad23fedda288507b87ae68dcfbe35b6a66285cf))
* adjust to new code place :bug: ([23ddb23](https://github.com/LerianStudio/midaz/commit/23ddb23d090ded59b060e546e067f85bfd7bf43f))
* adjust to remove .git :bug: ([02e65af](https://github.com/LerianStudio/midaz/commit/02e65afb450b5b369a27fd285a25b33e63f4a974))
* adjust to return nil and not empty struct :bug: ([a2a73b8](https://github.com/LerianStudio/midaz/commit/a2a73b851e2af5f43bfc445efdb565c281aef94c))
* adjust to run rabbit and fiber at same time :bug: ([4ec503f](https://github.com/LerianStudio/midaz/commit/4ec503fa0fa2a457b2c055d7585d80edba46cd48))
* adjust to test rabbit receiving data :bug: ([38d3ec9](https://github.com/LerianStudio/midaz/commit/38d3ec9908429171c9de4a772cb082dbdfdb17a8))
* adjust unit test :bug: ([da988f0](https://github.com/LerianStudio/midaz/commit/da988f0d3ee1937c680c197c8b29138281c306c2))
* always set true in isfrom json :bug: ([a497ed0](https://github.com/LerianStudio/midaz/commit/a497ed0b31c62798cd6b123b51be0c0c3c6ab581))
* audit routing key env name :bug: ([45482e9](https://github.com/LerianStudio/midaz/commit/45482e934fd55610e61a7e437741d7fd01ef3f9b))
* change env local :bug: ([e07b26e](https://github.com/LerianStudio/midaz/commit/e07b26e3a733a3fe75082f2ff79caa352248e1eb))
* **audit:** change log level for mtrillian :bug: ([06bd3f8](https://github.com/LerianStudio/midaz/commit/06bd3f8d55a84f3509bdcd5fa60ac7726d83cf5c))
* **audit:** change otel exporter service name :bug: ([85c15b4](https://github.com/LerianStudio/midaz/commit/85c15b45010d43c9bdd702b9d55e42186eb2b6d2))
* change place order :bug: ([96f416d](https://github.com/LerianStudio/midaz/commit/96f416d4feae874a976d2473771776a429655e02))
* change rabbit and mongo envs for audit component :bug: ([2854909](https://github.com/LerianStudio/midaz/commit/2854909dcb3a2f902fec9bdec923ad3d41d4ac9e))
* change to gh again :bug: ([4a3449b](https://github.com/LerianStudio/midaz/commit/4a3449b6f87b13359d8ac159eb4e11d6e481589d))
* check my place :bug: ([4b963bd](https://github.com/LerianStudio/midaz/commit/4b963bd722470e578c492e38d7485dcd2d1b0389))
* codeql :bug: ([1edae06](https://github.com/LerianStudio/midaz/commit/1edae06355e9c54c3687b0c460c8e2eebdb47ee7))
* **lint:** create and use func to safely converts int64 to int :bug: ([e9dc804](https://github.com/LerianStudio/midaz/commit/e9dc804e9163bdbeb5bfaabf75ed90d11c4addcc))
* exclude external from allowed account types for account creation :bug: ([18ec6ba](https://github.com/LerianStudio/midaz/commit/18ec6bab807943c03722a191229f609fbefb02c9))
* final adjusts :bug: ([fafa647](https://github.com/LerianStudio/midaz/commit/fafa6479916648aec7ea7c8ad13276250a0b0516))
* final version :bug: ([65d2656](https://github.com/LerianStudio/midaz/commit/65d26569969efabbc588c2e7c281e3ed85f96cfa))
* **audit:** fix field name :bug: ([eb8f647](https://github.com/LerianStudio/midaz/commit/eb8f647c45bcdf00776b4f57487d0ba7d0575cc2))
* **lint:** fix lint SA4003 :bug: ([5ac015d](https://github.com/LerianStudio/midaz/commit/5ac015de8ee747f9efe5cdd73990fa3c63ae6f6e))
* **lint:** fix lint SA4003 in os :bug: :bug: ([4b42f6a](https://github.com/LerianStudio/midaz/commit/4b42f6a52bdfa54f0b92e38ce4dff0db2d2d63fb))
* git actions swaggo :bug: ([246dd51](https://github.com/LerianStudio/midaz/commit/246dd51de7df189a422d2e27124de38287f95020))
* git clone :bug: ([7cc209a](https://github.com/LerianStudio/midaz/commit/7cc209a0f07f7c46f42469443fd79356409f7c43))
* go lint :bug: ([0123db1](https://github.com/LerianStudio/midaz/commit/0123db151fa218b044c189613f0b80cbc66aa105))
* **audit:** handle audit not found error only :bug: ([212ebac](https://github.com/LerianStudio/midaz/commit/212ebaca6c85dd12b150e273754648a805359710))
* **lint:** improve boolean tag validation return :bug: ([fef2192](https://github.com/LerianStudio/midaz/commit/fef219229eb167edaeba8c11ce0a8504ffff07b0))
* **audit:** make constants public :bug: ([baaee67](https://github.com/LerianStudio/midaz/commit/baaee675eaae47695a7dd00df93677bb5e60b0ff))
* merge git :bug: ([65a985a](https://github.com/LerianStudio/midaz/commit/65a985ac9c3758aaeca4fd861bb141fc095472f3))
* midaz-id header name :bug: ([ec79675](https://github.com/LerianStudio/midaz/commit/ec7967535065d79c8a7ef8d67497ef1d9a8bde09))
* more adjusts :bug: ([dfc3513](https://github.com/LerianStudio/midaz/commit/dfc351324a2fd6f6aebc4c72aab415dd1815a084))
* more adjusts and replace wrong calls :bug: ([ed7c57d](https://github.com/LerianStudio/midaz/commit/ed7c57d3af59154dffbcc95a84bb2ee355b94271))
* more shrink actions :bug: ([efa9e96](https://github.com/LerianStudio/midaz/commit/efa9e9694578d9a6fd00cf513d3e3fb0c7b88943))
* **audit:** nack message when an error occurs :bug: ([88090b0](https://github.com/LerianStudio/midaz/commit/88090b09b9377172ad77f4378139fae7441e0d04))
* new adjust :bug: ([2259b11](https://github.com/LerianStudio/midaz/commit/2259b1190024ae87911044bb4c5093b7ef81b319))
* **account:** omit optional fields in update request payload :bug: ([33f3e7d](https://github.com/LerianStudio/midaz/commit/33f3e7dac14088b8a6ff293ed4625eeef62a9448))
* **audit:** otel envs :bug: ([6328d90](https://github.com/LerianStudio/midaz/commit/6328d905a1f5f7e9dba5201ccd16fce3d884909a))
* rabbit init on server before fiber :bug: ([51c1b53](https://github.com/LerianStudio/midaz/commit/51c1b53eada3fd2cfbcc18557c101554607c74a1))
* remove BRL default :bug: ([912dce2](https://github.com/LerianStudio/midaz/commit/912dce2161ed9a78ef3faaf9bd48aa7f670a15e4))
* **ledger:** remove create audit tree from ledger creation :bug: ([8783145](https://github.com/LerianStudio/midaz/commit/878314570180bd8f1855572d435b37210e711218))
* remove G :bug: ([9fe64e1](https://github.com/LerianStudio/midaz/commit/9fe64e1a38aba6e851570e44b3aac8e1b61be795))
* remove is admin true from non admin users :bug: ([b02232f](https://github.com/LerianStudio/midaz/commit/b02232f5acf2fe3e5a80b70e06d7f22e44396be5))
* remove md5 and sha-256 generate string at this moment :bug: ([8d1adbd](https://github.com/LerianStudio/midaz/commit/8d1adbd91aa02068ce92d256e433327a097a775a))
* remove second queue consumer after tests :bug: ([8df4703](https://github.com/LerianStudio/midaz/commit/8df470377954add22e5ebb2422c69ee68931746c))
* remove workoing-directory :bug: ([b03b547](https://github.com/LerianStudio/midaz/commit/b03b547e7b1e48a9e0014c40b8350031c479f2d7))
* reorganize code :bug: ([54debfc](https://github.com/LerianStudio/midaz/commit/54debfc25a106e263962c94970fd8d21fa757d5a))
* return to root :bug: ([50b03d0](https://github.com/LerianStudio/midaz/commit/50b03d03f01dfaa87713dc4c75d5685a7c7e3e87))
* review dog fail_error to any :bug: ([f7a00f9](https://github.com/LerianStudio/midaz/commit/f7a00f98d557f517ac6295865ed439a8f6755c29))
* set token url remote :bug: ([acf4227](https://github.com/LerianStudio/midaz/commit/acf422701670f7688732b5b01d81bdab234194b5))
* **audit:** shutdown when consumer error :bug: ([22b24c9](https://github.com/LerianStudio/midaz/commit/22b24c90c67492d58ba4aa043f3a9d3513280777))
* some redefinitions :bug: ([5eae327](https://github.com/LerianStudio/midaz/commit/5eae3274dfefbd0b1d0a01c7d89acaa38146ab8c))
* swag --version :bug: ([bd0ab17](https://github.com/LerianStudio/midaz/commit/bd0ab17e47bdd569cafbbd5f1af48842803de099))
* swaggo install :bug: ([718c42e](https://github.com/LerianStudio/midaz/commit/718c42e52d7a585b7cbf8434f80dd2ab192f15ab))
* test :bug: ([7bf82f7](https://github.com/LerianStudio/midaz/commit/7bf82f76ba7a592837795786b8750d90ffbec98a))
* test :bug: ([b2e88f8](https://github.com/LerianStudio/midaz/commit/b2e88f8fedbd24dfddb42f478c6bae6b6c3e2c6a))
* test :bug: ([ca48838](https://github.com/LerianStudio/midaz/commit/ca48838e7f6786509292e0936bb8bacd8d824cfc))
* test :bug: ([481f4a8](https://github.com/LerianStudio/midaz/commit/481f4a89082b6471bcf4248f57f737d5bed3d3db))
* test :bug: ([f2889a2](https://github.com/LerianStudio/midaz/commit/f2889a2db28ead77d43874673376ab47cb104ba1))
* test :bug: ([c3b3313](https://github.com/LerianStudio/midaz/commit/c3b3313149a3bba19e3b4e2723dfacc533087785))
* test :bug: ([e51d1fd](https://github.com/LerianStudio/midaz/commit/e51d1fda2d264c22595d4306d179c65bce31325e))
* test :bug: ([cee71cb](https://github.com/LerianStudio/midaz/commit/cee71cb73d9ccffbde2263754110cd13e276812d))
* test :bug: ([dc865b1](https://github.com/LerianStudio/midaz/commit/dc865b11d8757a3937f4bf7c81fee69dfa5c201e))
* test :bug: ([a3fb8f0](https://github.com/LerianStudio/midaz/commit/a3fb8f01270799890df2a7614cf02e35a9ec8bec))
* test if make is installed :bug: ([81f3a1c](https://github.com/LerianStudio/midaz/commit/81f3a1caa34121649755558bedd0ea3697187ed0))
* **audit:** trillian server host name :bug: ([84b73ff](https://github.com/LerianStudio/midaz/commit/84b73ffdbae04d3c207155b976bab60d59e285ab))
* ubuntu version :bug: ([64748c7](https://github.com/LerianStudio/midaz/commit/64748c7e6b7e2f30577b550792f6a19b0861ad8d))
* unify codeql/lint/sec/unit :bug: ([53de44c](https://github.com/LerianStudio/midaz/commit/53de44c785ddb2c990735ec00036b5ad8eed94f5))
* update :bug: ([7e88db0](https://github.com/LerianStudio/midaz/commit/7e88db020132d0616380ba5bd433e93fecf317af))
* update :bug: ([ff843ac](https://github.com/LerianStudio/midaz/commit/ff843ac9570ce5aa9e7082857db1cf905d99b795))
* update :bug: ([a98be20](https://github.com/LerianStudio/midaz/commit/a98be2043979854d07750a49fedc684daadf5458))
* update :bug: ([1012a51](https://github.com/LerianStudio/midaz/commit/1012a51d18becf236b7333cb0b65c90ca03e905a))
* update :bug: ([8886a73](https://github.com/LerianStudio/midaz/commit/8886a73f7713db07e102adcaa8199535a6cdd972))
* update :bug: ([033a237](https://github.com/LerianStudio/midaz/commit/033a2371c105bb1db20a26020a3731bd9cd1a302))
* update :bug: ([b446031](https://github.com/LerianStudio/midaz/commit/b4460317a73f37e66b9d234db23fd9b4ab1dbf4d))
* update :bug: ([b320e46](https://github.com/LerianStudio/midaz/commit/b320e4629ad909b72fff63aea99cff066b33b5f1))
* update :bug: ([848cc1b](https://github.com/LerianStudio/midaz/commit/848cc1bf7af2008487135d065f9101a8cbb07ec1))
* update audit env version :bug: ([79475b2](https://github.com/LerianStudio/midaz/commit/79475b268aaac99b30f381a60cff4d41b5bfeffb))
* update check :bug: ([2c98e13](https://github.com/LerianStudio/midaz/commit/2c98e134acb3c80734f635eb667d5fdb985e7349))
* update check and name :bug: ([7533a52](https://github.com/LerianStudio/midaz/commit/7533a52d75c21fcd6c8888d9e1462db4537e4ddb))
* update checks :bug: ([7cf15ad](https://github.com/LerianStudio/midaz/commit/7cf15ad57a97460d0c0f0ff94d1aac81bcedec58))
* **audit:** update components/audit/internal/adapters/http/in/response.go ([3d8d8cd](https://github.com/LerianStudio/midaz/commit/3d8d8cd6ca41444132ef6682dede5e6f54859bc5))
* update error in yaml syntax :bug: ([322f2c9](https://github.com/LerianStudio/midaz/commit/322f2c9dcad4a98184a7dcd35614bbc5a79f9c4b))
* update error message when patching and deleting external accounts on ledger :bug: ([e0c8614](https://github.com/LerianStudio/midaz/commit/e0c8614d476475e6bc05806c27c84ad62bcac578))
* update folders paths :bug: ([18f872b](https://github.com/LerianStudio/midaz/commit/18f872b7eddd6e259a28e788ae9657c03caa1060))
* update image to ubuntu-24.04 :bug: ([b91d104](https://github.com/LerianStudio/midaz/commit/b91d10489deabb188beb37d583fc1976e970be96))
* **lint:** update incorrect conversion of a signed 64-bit integer to a lower bit size type int :bug: ([e0d8962](https://github.com/LerianStudio/midaz/commit/e0d896200f09c91041054019cf3f7546b5456443))
* **lint:** update incorrect conversion of int to use math min and math int constants :bug: ([02905b5](https://github.com/LerianStudio/midaz/commit/02905b51126e75fc68ccd11ce0ff3109740ed99f))
* update lint :bug: ([6494a72](https://github.com/LerianStudio/midaz/commit/6494a72def85575d860f611f9f5005021a57f76e))
* update make :bug: ([78effdc](https://github.com/LerianStudio/midaz/commit/78effdc4dbc58836d311eb671078626d05a08c61))
* update makefile to reference common to pkg :bug: ([c6963ea](https://github.com/LerianStudio/midaz/commit/c6963eae0776b3da149345e52f40649669adf02a))
* update place :bug: ([8d5501a](https://github.com/LerianStudio/midaz/commit/8d5501a2d39f6a8c3eef9592b6dc0e17be016781))
* update reference :bug: ([3d2f96f](https://github.com/LerianStudio/midaz/commit/3d2f96f91a663153b156283907aeb69f6196ebc8))
* update some checks :bug: ([f551b35](https://github.com/LerianStudio/midaz/commit/f551b35126688fbdcb0d9446f24e32ae818cf9b4))
* update to new approach :bug: ([bf6303d](https://github.com/LerianStudio/midaz/commit/bf6303d960c15a4f54c8cfcb0d6116236b1db2f1))
* using make file to generate swagger file :bug: ([9c9d545](https://github.com/LerianStudio/midaz/commit/9c9d5455f9eead5e95c91e722e6b02fef9f7530c))

## [1.31.0-beta.22](https://github.com/LerianStudio/midaz/compare/v1.31.0-beta.21...v1.31.0-beta.22) (2024-12-17)


### Features

*  finish redis verbs :sparkles: ([5ae2ddc](https://github.com/LerianStudio/midaz/commit/5ae2ddc8437b9be4fdaf7f8113cf4bb082aa16df))
* utils to convert string into hash to use on redis using idempotency :sparkles: ([9a64020](https://github.com/LerianStudio/midaz/commit/9a64020ea3da73eec9d7b7773cf12a7f2ea2e1ce))


### Bug Fixes

* always set true in isfrom json :bug: ([a497ed0](https://github.com/LerianStudio/midaz/commit/a497ed0b31c62798cd6b123b51be0c0c3c6ab581))
* remove BRL default :bug: ([912dce2](https://github.com/LerianStudio/midaz/commit/912dce2161ed9a78ef3faaf9bd48aa7f670a15e4))
* remove md5 and sha-256 generate string at this moment :bug: ([8d1adbd](https://github.com/LerianStudio/midaz/commit/8d1adbd91aa02068ce92d256e433327a097a775a))
* update lint :bug: ([6494a72](https://github.com/LerianStudio/midaz/commit/6494a72def85575d860f611f9f5005021a57f76e))

## [1.31.0-beta.21](https://github.com/LerianStudio/midaz/compare/v1.31.0-beta.20...v1.31.0-beta.21) (2024-12-17)


### Bug Fixes

* remove is admin true from non admin users :bug: ([b02232f](https://github.com/LerianStudio/midaz/commit/b02232f5acf2fe3e5a80b70e06d7f22e44396be5))

## [1.31.0-beta.20](https://github.com/LerianStudio/midaz/compare/v1.31.0-beta.19...v1.31.0-beta.20) (2024-12-12)

## [1.31.0-beta.19](https://github.com/LerianStudio/midaz/compare/v1.31.0-beta.18...v1.31.0-beta.19) (2024-12-12)


### Features

* **asset-rate:** add cursor pagination to get all entities endpoint :sparkles: ([441c51c](https://github.com/LerianStudio/midaz/commit/441c51c5e5d27672f2c87cfbf3b512b85bf38798))
* **transaction:** add cursor pagination to get all entities endpoint :sparkles: ([9b1cb94](https://github.com/LerianStudio/midaz/commit/9b1cb9405e6dc86a27aeb1b8980d9a68ce430734))
* **operation:** add cursor pagination to get all entities endpoints :sparkles: ([21315a3](https://github.com/LerianStudio/midaz/commit/21315a317cf0ed329900769980fa5fc3fb7f17ce))
* **postman:** add pagination fields to postman for get all endpoints :sparkles: ([63e3e56](https://github.com/LerianStudio/midaz/commit/63e3e56b033cbddc8edb01d986c5e37c6d060834))
* **pagination:** add sort order filter and date ranges to the midaz pagination filtering :sparkles: ([4cc01d3](https://github.com/LerianStudio/midaz/commit/4cc01d311f51c16b759d7e8e1e287193eafab0d8))
* **pagination:** improve pagination validations and tests :sparkles: ([8226e87](https://github.com/LerianStudio/midaz/commit/8226e87338c1a847c85301cd3752420e2b8cb1a7))
* **tests:** update tests with to pagination filter struct :sparkles: ([793b685](https://github.com/LerianStudio/midaz/commit/793b685541ebcb5c3897d585380f16d2f9705d37))


### Bug Fixes

* **lint:** create and use func to safely converts int64 to int :bug: ([e9dc804](https://github.com/LerianStudio/midaz/commit/e9dc804e9163bdbeb5bfaabf75ed90d11c4addcc))
* **lint:** fix lint SA4003 :bug: ([5ac015d](https://github.com/LerianStudio/midaz/commit/5ac015de8ee747f9efe5cdd73990fa3c63ae6f6e))
* **lint:** fix lint SA4003 in os :bug: :bug: ([4b42f6a](https://github.com/LerianStudio/midaz/commit/4b42f6a52bdfa54f0b92e38ce4dff0db2d2d63fb))
* **lint:** update incorrect conversion of a signed 64-bit integer to a lower bit size type int :bug: ([e0d8962](https://github.com/LerianStudio/midaz/commit/e0d896200f09c91041054019cf3f7546b5456443))
* **lint:** update incorrect conversion of int to use math min and math int constants :bug: ([02905b5](https://github.com/LerianStudio/midaz/commit/02905b51126e75fc68ccd11ce0ff3109740ed99f))

## [1.31.0-beta.18](https://github.com/LerianStudio/midaz/compare/v1.31.0-beta.17...v1.31.0-beta.18) (2024-12-12)

## [1.31.0-beta.17](https://github.com/LerianStudio/midaz/compare/v1.31.0-beta.16...v1.31.0-beta.17) (2024-12-11)


### Features

* added new flags to get list filters :sparkles: ([959cc9d](https://github.com/LerianStudio/midaz/commit/959cc9db71a40b963af279be17e8be48aa79b123))

## [1.31.0-beta.16](https://github.com/LerianStudio/midaz/compare/v1.31.0-beta.15...v1.31.0-beta.16) (2024-12-11)


### Features

* async log transaction call :sparkles: ([35816e4](https://github.com/LerianStudio/midaz/commit/35816e444d153a4e555ab5708a20d3635ffe69da))
* create operation log struct to specify fields that should be immutable :sparkles: ([b5438c1](https://github.com/LerianStudio/midaz/commit/b5438c15eba68b1e35683a41507eb5105cafa140))
* disable audit logging thought env :sparkles: ([8fa77c8](https://github.com/LerianStudio/midaz/commit/8fa77c8871073c613c55f695722ffcf15240a17a))
* get audit exchange and routing key names from envs :sparkles: ([ce70e91](https://github.com/LerianStudio/midaz/commit/ce70e9106c715a18225c4cf50f234b891de46dc0))
* remove correlation-id after midaz-id implemented :sparkles: ([63e8016](https://github.com/LerianStudio/midaz/commit/63e80169edfabba82848b61d56486866b8763c1f))
* **ledger:** remove exchange and key from connection :sparkles: ([621cbf9](https://github.com/LerianStudio/midaz/commit/621cbf949446f216aae08f5b9bead44afb90c01e))
* remove exchange and key from rabbitmq connection config :sparkles: ([aa086a1](https://github.com/LerianStudio/midaz/commit/aa086a160badcb6f57f589ffc2a5315db2a35e13))
* send log message after transaction created :sparkles: ([66f3f64](https://github.com/LerianStudio/midaz/commit/66f3f64065654f1bcc292e458edb667a2296b5e5))
* update producer to receive Queue message, exchange and key through parameters :sparkles: ([8dc41f3](https://github.com/LerianStudio/midaz/commit/8dc41f3f94935297506ff12507626329ea52d669))
* **ledger:** update rabbitmq producer :sparkles: ([47e3eef](https://github.com/LerianStudio/midaz/commit/47e3eef87a62b87fdc61e9564e6e5bc5c7f9da2a))


### Bug Fixes

* audit routing key env name :bug: ([45482e9](https://github.com/LerianStudio/midaz/commit/45482e934fd55610e61a7e437741d7fd01ef3f9b))
* midaz-id header name :bug: ([ec79675](https://github.com/LerianStudio/midaz/commit/ec7967535065d79c8a7ef8d67497ef1d9a8bde09))

## [1.31.0-beta.15](https://github.com/LerianStudio/midaz/compare/v1.31.0-beta.14...v1.31.0-beta.15) (2024-12-10)

## [1.31.0-beta.14](https://github.com/LerianStudio/midaz/compare/v1.31.0-beta.13...v1.31.0-beta.14) (2024-12-06)


### Features

* **audit:** add audit logs handler :sparkles: ([4a5fe36](https://github.com/LerianStudio/midaz/commit/4a5fe36a69fb9342d962c07a5fafdb64bbdfcfa4))
* **audit:** add authorization for routes :sparkles: ([2700d50](https://github.com/LerianStudio/midaz/commit/2700d50d93fbdb2203dc8ef19335d26f86737e45))
* **audit:** add custom error messages :sparkles: ([db9bc72](https://github.com/LerianStudio/midaz/commit/db9bc72195faa6bbb6d143260baa34e0db7d032c))
* **audit:** add get audit info use case :sparkles: ([9cc6503](https://github.com/LerianStudio/midaz/commit/9cc65035dd99edb6a1626acc67efec0d1fad108d))
* **audit:** add get log by hash use case :sparkles: ([66d3b93](https://github.com/LerianStudio/midaz/commit/66d3b9379ac47475f9f32f9fe70e1c52ce9d46b7))
* **audit:** add methods for retrieving trillian inclusion proof and leaf by index :sparkles: ([03b12bd](https://github.com/LerianStudio/midaz/commit/03b12bdd406cab91295d0bd21de96574f8c09e53))
* **audit:** add trace spans :sparkles: ([1ea30fa](https://github.com/LerianStudio/midaz/commit/1ea30fab9d2c75bebd51309d709a9b833d0b66d4))
* **audit:** add trillian health check before connecting :sparkles: ([9295cec](https://github.com/LerianStudio/midaz/commit/9295cec1036dd77da7d843c38603247be2d46ed5))
* add update swagger audit on git pages ([137824a](https://github.com/LerianStudio/midaz/commit/137824a9f721e140a4ecb7ec08cca07c99762b59))
* **audit:** add validate log use case :sparkles: ([7216c5e](https://github.com/LerianStudio/midaz/commit/7216c5e744d0246961db040f6c045c60452b1dc1))
* audit component :sparkles: ([084603f](https://github.com/LerianStudio/midaz/commit/084603f08386b7ebcfa67eaac7b094ddf676976f))
* **audit:** audit structs to aux mongo database ([4b80b75](https://github.com/LerianStudio/midaz/commit/4b80b75a16cefb77a4908e04b7ac522e347fb8eb))
* create audit app ([f3f8cd5](https://github.com/LerianStudio/midaz/commit/f3f8cd5f3e7e8023e17e1f17111e9e221ec62227))
* **auth:** create auditor user :sparkles: ([5953ad9](https://github.com/LerianStudio/midaz/commit/5953ad9ac44faa3c8c9014eb47d480176f6d49ca))
* create route consumer for many queues ([8004063](https://github.com/LerianStudio/midaz/commit/8004063186d6c85bd0ed99e5d081acdc9ecdfb8f))
* **audit:** create struct for queue messages :sparkles: ([646bd38](https://github.com/LerianStudio/midaz/commit/646bd38cced4fc57f51fb2e5bd3d3137ba2a83bc))
* **audit:** create structs for audit transaction message ([fa6b568](https://github.com/LerianStudio/midaz/commit/fa6b568d83b165d59540ee7878e550f81ddc3789))
* **audit:** create transaction logs from rabbitmq message ([d54e4d3](https://github.com/LerianStudio/midaz/commit/d54e4d387e08ea5d9a47898bd0e94df7ab5c2f5d))
* **audit:** create trillian log leaf ([d18c0c2](https://github.com/LerianStudio/midaz/commit/d18c0c22f540196e575bfc1f0656da1fb5747a54))
* **audit:** errors return from log creation :sparkles: ([69594e4](https://github.com/LerianStudio/midaz/commit/69594e4e24eb6d107b2d8fa27f83d0e76e058405))
* **audit:** find audit info by ID :sparkles: ([ea91e97](https://github.com/LerianStudio/midaz/commit/ea91e971ac2db8cd8a7befe2e42d994e6987902f))
* **audit:** ignore updatable fields for operation :sparkles: ([28db38d](https://github.com/LerianStudio/midaz/commit/28db38d0e391904458fd8234303f6f56b412e6c3))
* **audit:** implement get trillian log by hash :sparkles: ([44d103b](https://github.com/LerianStudio/midaz/commit/44d103bbef1e80acd37ecd5c5e3d4ce238ea8530))
* **audit:** implement rabbitmq consumer :sparkles: ([9874dc4](https://github.com/LerianStudio/midaz/commit/9874dc453cfcbb94379c9e256c6aeeacef136bc9))
* implement trillian connection :sparkles: ([c4b8877](https://github.com/LerianStudio/midaz/commit/c4b887706dd4ce4ea8c7f7358ff40854f60bc2a6))
* **audit:** implements read logs by transaction handler :sparkles: ([d134b07](https://github.com/LerianStudio/midaz/commit/d134b07e7715f05b9e32817a47fe95ded1721c7b))
* **audit:** receiving audit parameter to create tree ([be43f32](https://github.com/LerianStudio/midaz/commit/be43f324ac21354c60a65ce2beda5f1c4f78871f))
* **audit:** returning log leaf instead of the value :sparkles: ([9b40d88](https://github.com/LerianStudio/midaz/commit/9b40d88189c3021e637cc3ce52686895b5b83130))
* right way of starter audit with only one queue consumer ([15a0a8c](https://github.com/LerianStudio/midaz/commit/15a0a8c9438d31597d749d3180adcd4a9eb994bc))
* **audit:** starting implementation of server :sparkles: ([edbce7b](https://github.com/LerianStudio/midaz/commit/edbce7bc2281c7d1273215dc372573e58680119c))
* **audit:** update audit info collection name :sparkles: ([7cd39fa](https://github.com/LerianStudio/midaz/commit/7cd39fa0861b06c7a728f9c25e44f656d2be7b50))
* **audit:** update audit route paths :sparkles: ([0f12899](https://github.com/LerianStudio/midaz/commit/0f128998b6525c4419e3e4acd388aac97e92cb48))
* **audit:** using generic queue struct instead of transaction to write logs :sparkles: ([4c1b86f](https://github.com/LerianStudio/midaz/commit/4c1b86f0f374d182ee39b430ea19b641bad4eca0))
* **audit:** work with generic audit log values :sparkles: ([9beb218](https://github.com/LerianStudio/midaz/commit/9beb21876f2cc57aacaabee502c45712e68102db))


### Bug Fixes

* **audit:** add audit_id parameter to uuid path parameters constant :bug: ([dcbcb05](https://github.com/LerianStudio/midaz/commit/dcbcb05de4d2f1cfb9340a807f299af6bb302c5f))
* **audit:** add tree size validation to fix vulnerability :bug: ([313dbf4](https://github.com/LerianStudio/midaz/commit/313dbf40f06d088e2d36282f57a7585db3e5ab7a))
* adjust to change run :bug: ([bad23fe](https://github.com/LerianStudio/midaz/commit/bad23fedda288507b87ae68dcfbe35b6a66285cf))
* adjust to run rabbit and fiber at same time :bug: ([4ec503f](https://github.com/LerianStudio/midaz/commit/4ec503fa0fa2a457b2c055d7585d80edba46cd48))
* adjust to test rabbit receiving data :bug: ([38d3ec9](https://github.com/LerianStudio/midaz/commit/38d3ec9908429171c9de4a772cb082dbdfdb17a8))
* **audit:** change log level for mtrillian :bug: ([06bd3f8](https://github.com/LerianStudio/midaz/commit/06bd3f8d55a84f3509bdcd5fa60ac7726d83cf5c))
* **audit:** change otel exporter service name :bug: ([85c15b4](https://github.com/LerianStudio/midaz/commit/85c15b45010d43c9bdd702b9d55e42186eb2b6d2))
* change rabbit and mongo envs for audit component :bug: ([2854909](https://github.com/LerianStudio/midaz/commit/2854909dcb3a2f902fec9bdec923ad3d41d4ac9e))
* **audit:** fix field name :bug: ([eb8f647](https://github.com/LerianStudio/midaz/commit/eb8f647c45bcdf00776b4f57487d0ba7d0575cc2))
* **audit:** handle audit not found error only :bug: ([212ebac](https://github.com/LerianStudio/midaz/commit/212ebaca6c85dd12b150e273754648a805359710))
* **audit:** make constants public :bug: ([baaee67](https://github.com/LerianStudio/midaz/commit/baaee675eaae47695a7dd00df93677bb5e60b0ff))
* merge git :bug: ([65a985a](https://github.com/LerianStudio/midaz/commit/65a985ac9c3758aaeca4fd861bb141fc095472f3))
* **audit:** nack message when an error occurs :bug: ([88090b0](https://github.com/LerianStudio/midaz/commit/88090b09b9377172ad77f4378139fae7441e0d04))
* **audit:** otel envs :bug: ([6328d90](https://github.com/LerianStudio/midaz/commit/6328d905a1f5f7e9dba5201ccd16fce3d884909a))
* rabbit init on server before fiber :bug: ([51c1b53](https://github.com/LerianStudio/midaz/commit/51c1b53eada3fd2cfbcc18557c101554607c74a1))
* **ledger:** remove create audit tree from ledger creation :bug: ([8783145](https://github.com/LerianStudio/midaz/commit/878314570180bd8f1855572d435b37210e711218))
* remove second queue consumer after tests :bug: ([8df4703](https://github.com/LerianStudio/midaz/commit/8df470377954add22e5ebb2422c69ee68931746c))
* **audit:** shutdown when consumer error :bug: ([22b24c9](https://github.com/LerianStudio/midaz/commit/22b24c90c67492d58ba4aa043f3a9d3513280777))
* **audit:** trillian server host name :bug: ([84b73ff](https://github.com/LerianStudio/midaz/commit/84b73ffdbae04d3c207155b976bab60d59e285ab))
* update audit env version :bug: ([79475b2](https://github.com/LerianStudio/midaz/commit/79475b268aaac99b30f381a60cff4d41b5bfeffb))
* **audit:** update components/audit/internal/adapters/http/in/response.go ([3d8d8cd](https://github.com/LerianStudio/midaz/commit/3d8d8cd6ca41444132ef6682dede5e6f54859bc5))

## [1.31.0-beta.13](https://github.com/LerianStudio/midaz/compare/v1.31.0-beta.12...v1.31.0-beta.13) (2024-12-06)


### Bug Fixes

* adjust to return nil and not empty struct :bug: ([a2a73b8](https://github.com/LerianStudio/midaz/commit/a2a73b851e2af5f43bfc445efdb565c281aef94c))
* go lint :bug: ([0123db1](https://github.com/LerianStudio/midaz/commit/0123db151fa218b044c189613f0b80cbc66aa105))
* remove G :bug: ([9fe64e1](https://github.com/LerianStudio/midaz/commit/9fe64e1a38aba6e851570e44b3aac8e1b61be795))
* update makefile to reference common to pkg :bug: ([c6963ea](https://github.com/LerianStudio/midaz/commit/c6963eae0776b3da149345e52f40649669adf02a))

## [1.31.0-beta.12](https://github.com/LerianStudio/midaz/compare/v1.31.0-beta.11...v1.31.0-beta.12) (2024-12-06)

## [1.31.0-beta.11](https://github.com/LerianStudio/midaz/compare/v1.31.0-beta.10...v1.31.0-beta.11) (2024-12-06)


### Bug Fixes

* **account:** omit optional fields in update request payload :bug: ([33f3e7d](https://github.com/LerianStudio/midaz/commit/33f3e7dac14088b8a6ff293ed4625eeef62a9448))

## [1.31.0-beta.10](https://github.com/LerianStudio/midaz/compare/v1.31.0-beta.9...v1.31.0-beta.10) (2024-12-06)

## [1.31.0-beta.9](https://github.com/LerianStudio/midaz/compare/v1.31.0-beta.8...v1.31.0-beta.9) (2024-12-04)


### Bug Fixes

* add more actions with same background :bug: ([cdd8164](https://github.com/LerianStudio/midaz/commit/cdd8164c08f51e1d421eb00f67f46077ffcd35e4))
* add more rules and shrink actions :bug: ([ce2b916](https://github.com/LerianStudio/midaz/commit/ce2b916599073f9baea9c11d2860b2c77c712523))
* adjust unit test :bug: ([da988f0](https://github.com/LerianStudio/midaz/commit/da988f0d3ee1937c680c197c8b29138281c306c2))
* change place order :bug: ([96f416d](https://github.com/LerianStudio/midaz/commit/96f416d4feae874a976d2473771776a429655e02))
* codeql :bug: ([1edae06](https://github.com/LerianStudio/midaz/commit/1edae06355e9c54c3687b0c460c8e2eebdb47ee7))
* final version :bug: ([65d2656](https://github.com/LerianStudio/midaz/commit/65d26569969efabbc588c2e7c281e3ed85f96cfa))
* more adjusts :bug: ([dfc3513](https://github.com/LerianStudio/midaz/commit/dfc351324a2fd6f6aebc4c72aab415dd1815a084))
* more adjusts and replace wrong calls :bug: ([ed7c57d](https://github.com/LerianStudio/midaz/commit/ed7c57d3af59154dffbcc95a84bb2ee355b94271))
* more shrink actions :bug: ([efa9e96](https://github.com/LerianStudio/midaz/commit/efa9e9694578d9a6fd00cf513d3e3fb0c7b88943))
* new adjust :bug: ([2259b11](https://github.com/LerianStudio/midaz/commit/2259b1190024ae87911044bb4c5093b7ef81b319))
* reorganize code :bug: ([54debfc](https://github.com/LerianStudio/midaz/commit/54debfc25a106e263962c94970fd8d21fa757d5a))
* review dog fail_error to any :bug: ([f7a00f9](https://github.com/LerianStudio/midaz/commit/f7a00f98d557f517ac6295865ed439a8f6755c29))
* some redefinitions :bug: ([5eae327](https://github.com/LerianStudio/midaz/commit/5eae3274dfefbd0b1d0a01c7d89acaa38146ab8c))
* ubuntu version :bug: ([64748c7](https://github.com/LerianStudio/midaz/commit/64748c7e6b7e2f30577b550792f6a19b0861ad8d))
* unify codeql/lint/sec/unit :bug: ([53de44c](https://github.com/LerianStudio/midaz/commit/53de44c785ddb2c990735ec00036b5ad8eed94f5))
* update check :bug: ([2c98e13](https://github.com/LerianStudio/midaz/commit/2c98e134acb3c80734f635eb667d5fdb985e7349))
* update check and name :bug: ([7533a52](https://github.com/LerianStudio/midaz/commit/7533a52d75c21fcd6c8888d9e1462db4537e4ddb))
* update checks :bug: ([7cf15ad](https://github.com/LerianStudio/midaz/commit/7cf15ad57a97460d0c0f0ff94d1aac81bcedec58))
* update error in yaml syntax :bug: ([322f2c9](https://github.com/LerianStudio/midaz/commit/322f2c9dcad4a98184a7dcd35614bbc5a79f9c4b))
* update image to ubuntu-24.04 :bug: ([b91d104](https://github.com/LerianStudio/midaz/commit/b91d10489deabb188beb37d583fc1976e970be96))
* update reference :bug: ([3d2f96f](https://github.com/LerianStudio/midaz/commit/3d2f96f91a663153b156283907aeb69f6196ebc8))
* update some checks :bug: ([f551b35](https://github.com/LerianStudio/midaz/commit/f551b35126688fbdcb0d9446f24e32ae818cf9b4))

## [1.31.0-beta.8](https://github.com/LerianStudio/midaz/compare/v1.31.0-beta.7...v1.31.0-beta.8) (2024-12-03)


### Features

* check diff before commit changes ([4e5d2d3](https://github.com/LerianStudio/midaz/commit/4e5d2d3e3ac09cbd7819fdba7ba2eed24ff975ff))
* copy swagger.josn and check diff ([1cd0658](https://github.com/LerianStudio/midaz/commit/1cd0658dacd9747d4bd08b6d3f5b1e742791d115))
* generate swagger on midaz ([3678070](https://github.com/LerianStudio/midaz/commit/3678070fbf0f105359ec0206aed8cbacd26f5e06))
* test by specific branch ([a0f7af3](https://github.com/LerianStudio/midaz/commit/a0f7af3613d42ef23bf9f5f250a1fe7e58c7155a))
* update git pages ([1c6f8cc](https://github.com/LerianStudio/midaz/commit/1c6f8ccb098563a8ad2940a192cdcc6903ed686a))
* update pages with each json swagger ([b4d8563](https://github.com/LerianStudio/midaz/commit/b4d856369d400a829a0510ae02801c8f69d62b4b))
* update swagger to teste commit ([b6aa4bf](https://github.com/LerianStudio/midaz/commit/b6aa4bfcd42a06cac72ccb7f3ab766024ea23315))
* valida if has changes ([ac7cbdb](https://github.com/LerianStudio/midaz/commit/ac7cbdbc2bb621c9ff8c38bb4f407a86279c0f96))


### Bug Fixes

* adjust path :bug: ([41ec839](https://github.com/LerianStudio/midaz/commit/41ec839fc9a792229503f036b4e6e267cb8010cd))
* adjust to remove .git :bug: ([02e65af](https://github.com/LerianStudio/midaz/commit/02e65afb450b5b369a27fd285a25b33e63f4a974))
* change env local :bug: ([e07b26e](https://github.com/LerianStudio/midaz/commit/e07b26e3a733a3fe75082f2ff79caa352248e1eb))
* change to gh again :bug: ([4a3449b](https://github.com/LerianStudio/midaz/commit/4a3449b6f87b13359d8ac159eb4e11d6e481589d))
* check my place :bug: ([4b963bd](https://github.com/LerianStudio/midaz/commit/4b963bd722470e578c492e38d7485dcd2d1b0389))
* final adjusts :bug: ([fafa647](https://github.com/LerianStudio/midaz/commit/fafa6479916648aec7ea7c8ad13276250a0b0516))
* git actions swaggo :bug: ([246dd51](https://github.com/LerianStudio/midaz/commit/246dd51de7df189a422d2e27124de38287f95020))
* git clone :bug: ([7cc209a](https://github.com/LerianStudio/midaz/commit/7cc209a0f07f7c46f42469443fd79356409f7c43))
* remove workoing-directory :bug: ([b03b547](https://github.com/LerianStudio/midaz/commit/b03b547e7b1e48a9e0014c40b8350031c479f2d7))
* return to root :bug: ([50b03d0](https://github.com/LerianStudio/midaz/commit/50b03d03f01dfaa87713dc4c75d5685a7c7e3e87))
* set token url remote :bug: ([acf4227](https://github.com/LerianStudio/midaz/commit/acf422701670f7688732b5b01d81bdab234194b5))
* swag --version :bug: ([bd0ab17](https://github.com/LerianStudio/midaz/commit/bd0ab17e47bdd569cafbbd5f1af48842803de099))
* swaggo install :bug: ([718c42e](https://github.com/LerianStudio/midaz/commit/718c42e52d7a585b7cbf8434f80dd2ab192f15ab))
* test :bug: ([7bf82f7](https://github.com/LerianStudio/midaz/commit/7bf82f76ba7a592837795786b8750d90ffbec98a))
* test :bug: ([b2e88f8](https://github.com/LerianStudio/midaz/commit/b2e88f8fedbd24dfddb42f478c6bae6b6c3e2c6a))
* test :bug: ([ca48838](https://github.com/LerianStudio/midaz/commit/ca48838e7f6786509292e0936bb8bacd8d824cfc))
* test :bug: ([481f4a8](https://github.com/LerianStudio/midaz/commit/481f4a89082b6471bcf4248f57f737d5bed3d3db))
* test :bug: ([f2889a2](https://github.com/LerianStudio/midaz/commit/f2889a2db28ead77d43874673376ab47cb104ba1))
* test :bug: ([c3b3313](https://github.com/LerianStudio/midaz/commit/c3b3313149a3bba19e3b4e2723dfacc533087785))
* test :bug: ([e51d1fd](https://github.com/LerianStudio/midaz/commit/e51d1fda2d264c22595d4306d179c65bce31325e))
* test :bug: ([cee71cb](https://github.com/LerianStudio/midaz/commit/cee71cb73d9ccffbde2263754110cd13e276812d))
* test :bug: ([dc865b1](https://github.com/LerianStudio/midaz/commit/dc865b11d8757a3937f4bf7c81fee69dfa5c201e))
* test :bug: ([a3fb8f0](https://github.com/LerianStudio/midaz/commit/a3fb8f01270799890df2a7614cf02e35a9ec8bec))
* test if make is installed :bug: ([81f3a1c](https://github.com/LerianStudio/midaz/commit/81f3a1caa34121649755558bedd0ea3697187ed0))
* update :bug: ([7e88db0](https://github.com/LerianStudio/midaz/commit/7e88db020132d0616380ba5bd433e93fecf317af))
* update :bug: ([ff843ac](https://github.com/LerianStudio/midaz/commit/ff843ac9570ce5aa9e7082857db1cf905d99b795))
* update :bug: ([a98be20](https://github.com/LerianStudio/midaz/commit/a98be2043979854d07750a49fedc684daadf5458))
* update :bug: ([1012a51](https://github.com/LerianStudio/midaz/commit/1012a51d18becf236b7333cb0b65c90ca03e905a))
* update :bug: ([8886a73](https://github.com/LerianStudio/midaz/commit/8886a73f7713db07e102adcaa8199535a6cdd972))
* update :bug: ([033a237](https://github.com/LerianStudio/midaz/commit/033a2371c105bb1db20a26020a3731bd9cd1a302))
* update :bug: ([b446031](https://github.com/LerianStudio/midaz/commit/b4460317a73f37e66b9d234db23fd9b4ab1dbf4d))
* update :bug: ([b320e46](https://github.com/LerianStudio/midaz/commit/b320e4629ad909b72fff63aea99cff066b33b5f1))
* update :bug: ([848cc1b](https://github.com/LerianStudio/midaz/commit/848cc1bf7af2008487135d065f9101a8cbb07ec1))
* update folders paths :bug: ([18f872b](https://github.com/LerianStudio/midaz/commit/18f872b7eddd6e259a28e788ae9657c03caa1060))
* update make :bug: ([78effdc](https://github.com/LerianStudio/midaz/commit/78effdc4dbc58836d311eb671078626d05a08c61))
* update place :bug: ([8d5501a](https://github.com/LerianStudio/midaz/commit/8d5501a2d39f6a8c3eef9592b6dc0e17be016781))
* update to new approach :bug: ([bf6303d](https://github.com/LerianStudio/midaz/commit/bf6303d960c15a4f54c8cfcb0d6116236b1db2f1))
* using make file to generate swagger file :bug: ([9c9d545](https://github.com/LerianStudio/midaz/commit/9c9d5455f9eead5e95c91e722e6b02fef9f7530c))

## [1.31.0-beta.7](https://github.com/LerianStudio/midaz/compare/v1.31.0-beta.6...v1.31.0-beta.7) (2024-12-03)


### Features

* soft delete asset and its external account :sparkles: ([7b090ba](https://github.com/LerianStudio/midaz/commit/7b090baf368be777a23c26e09e2ee33a0bbc4e91))


### Bug Fixes

* **account:** add error message translation for prohibited external account creation and adjust validation assertion :bug: ([fdd5971](https://github.com/LerianStudio/midaz/commit/fdd59717c8cc8e419817ddea145a91ef7601d35a))
* **lint:** improve boolean tag validation return :bug: ([fef2192](https://github.com/LerianStudio/midaz/commit/fef219229eb167edaeba8c11ce0a8504ffff07b0))

## [1.31.0-beta.6](https://github.com/LerianStudio/midaz/compare/v1.31.0-beta.5...v1.31.0-beta.6) (2024-12-02)


### Bug Fixes

* adjust filter by metadata on get all transactions endpoint :bug: ([18c93a7](https://github.com/LerianStudio/midaz/commit/18c93a77b59d4e5d34d50d293534eebae3e22f60))

## [1.31.0-beta.5](https://github.com/LerianStudio/midaz/compare/v1.31.0-beta.4...v1.31.0-beta.5) (2024-12-02)


### Bug Fixes

* add slash to the forbidden account external aliases :bug: ([5e28fd5](https://github.com/LerianStudio/midaz/commit/5e28fd56fa2a61a2566a07690db97c01163561f3))
* add validation to patch and delete methods for external accounts on ledger :bug: ([96ba359](https://github.com/LerianStudio/midaz/commit/96ba359993badc9456ea9d9de9286e33a9b051aa))
* update error message when patching and deleting external accounts on ledger :bug: ([e0c8614](https://github.com/LerianStudio/midaz/commit/e0c8614d476475e6bc05806c27c84ad62bcac578))

## [1.31.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.31.0-beta.3...v1.31.0-beta.4) (2024-11-29)


### Bug Fixes

* exclude external from allowed account types for account creation :bug: ([18ec6ba](https://github.com/LerianStudio/midaz/commit/18ec6bab807943c03722a191229f609fbefb02c9))

## [1.31.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.31.0-beta.2...v1.31.0-beta.3) (2024-11-29)


### Features

* added command configure :sparkles: ([f269cf3](https://github.com/LerianStudio/midaz/commit/f269cf3c6a9f3badd2cea2bf93982433ff72e4af))
* configure command created defines the envs variables used in ldflags via command with the unit test of the ending with print command and print fields :sparkles: ([f407ab8](https://github.com/LerianStudio/midaz/commit/f407ab85224d30aa9f923dd27f9f49e76669e3d4))

## [1.31.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.31.0-beta.1...v1.31.0-beta.2) (2024-11-28)


### Bug Fixes

* add get git token to get tag version :bug: ([92b91e6](https://github.com/LerianStudio/midaz/commit/92b91e6c9306568e7a48a95311e82ef8a2ce2463))

## [1.31.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.30.0...v1.31.0-beta.1) (2024-11-28)


### Features

* steps to send slack message with release ([8957369](https://github.com/LerianStudio/midaz/commit/89573696f68c0a0ab20013cd265ea09874f02da5))


### Bug Fixes

* adjust to new code place :bug: ([23ddb23](https://github.com/LerianStudio/midaz/commit/23ddb23d090ded59b060e546e067f85bfd7bf43f))

## [1.30.0](https://github.com/LerianStudio/midaz/compare/v1.29.0...v1.30.0) (2024-11-28)


### Features

* format output colors and set flag global no-color :sparkles: ([7fae4c0](https://github.com/LerianStudio/midaz/commit/7fae4c044e1f060cbafbc751c2fa9c00fd60f308))


### Bug Fixes

* remove slack release notification :bug: ([de07047](https://github.com/LerianStudio/midaz/commit/de0704713e601d8c5a06198bc46a66f433ebc711))

## [1.30.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.30.0-beta.3...v1.30.0-beta.4) (2024-11-28)


### Bug Fixes

* remove slack release notification :bug: ([de07047](https://github.com/LerianStudio/midaz/commit/de0704713e601d8c5a06198bc46a66f433ebc711))

## [1.30.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.30.0-beta.2...v1.30.0-beta.3) (2024-11-28)

## [1.30.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.30.0-beta.1...v1.30.0-beta.2) (2024-11-27)

## [1.30.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.29.0...v1.30.0-beta.1) (2024-11-27)


### Features

* format output colors and set flag global no-color :sparkles: ([7fae4c0](https://github.com/LerianStudio/midaz/commit/7fae4c044e1f060cbafbc751c2fa9c00fd60f308))

## [1.29.0](https://github.com/LerianStudio/midaz/compare/v1.28.0...v1.29.0) (2024-11-26)


### Features

* add :sparkles: ([8baab22](https://github.com/LerianStudio/midaz/commit/8baab221b425c84fc56ee1eadcb8da3d09048543))
* add base to the swagger documentation and telemetry root span handling for the swagger endpoint calls :sparkles: ([0165a7c](https://github.com/LerianStudio/midaz/commit/0165a7c996a59e5941a2448e03e461b57088a677))
* add blocked to open pr to main if not come from develop or hotfix :sparkles: ([327448d](https://github.com/LerianStudio/midaz/commit/327448dafbd03db064c0f9488c0950e270d6556f))
* add reviewdog :sparkles: ([e5af335](https://github.com/LerianStudio/midaz/commit/e5af335e030c4e1ee7c68ec7ba6997db7c56cd4c))
* add reviewdog again :sparkles: ([3636404](https://github.com/LerianStudio/midaz/commit/363640416c1c263238ab8e3634f90cef348b8c5e))
* add rule to pr :sparkles: ([6e0ff0c](https://github.com/LerianStudio/midaz/commit/6e0ff0c010ea23feb1e3140ebe8e88abca2ae547))
* add swagger documentation generated for ledger :sparkles: ([cef9e22](https://github.com/LerianStudio/midaz/commit/cef9e22ee6558dc16372ab17e688129a5856212c))
* add swagger documentation to onboarding context on ledger service :sparkles: ([65ea499](https://github.com/LerianStudio/midaz/commit/65ea499a50e17f6e22f52f9705a833e4d64a134a))
* add swagger documentation to the portfolio context on ledger service :sparkles: ([fad4b08](https://github.com/LerianStudio/midaz/commit/fad4b08dbb7a0ee47f5b784ccef668d2843bab4f))
* add swagger documentation to transaction service :sparkles: ([e06a30e](https://github.com/LerianStudio/midaz/commit/e06a30e360e70079ce66c7f3aeecdd5536c8b134))
* add swagger generated docs from transaction :sparkles: ([a6e3775](https://github.com/LerianStudio/midaz/commit/a6e377576673c4a2c0a2691f717518d9ade65e0f))
* add version endpoint to ledger and transaction services :sparkles: ([bb646b7](https://github.com/LerianStudio/midaz/commit/bb646b75161b1698adacc32164862d910fa5e987))
* added command account in root ([7e2a439](https://github.com/LerianStudio/midaz/commit/7e2a439a26efa5786a5352b09875339d7545b2e6))
* added command describe from products ([4b4a222](https://github.com/LerianStudio/midaz/commit/4b4a22273e009760e2819b04063a8715388fdfa1))
* added command list from products ([fe7503e](https://github.com/LerianStudio/midaz/commit/fe7503ea6c4b971be4ffba55ed21035bfeb15710))
* added sub command create in commmand account with test unit ([29a424c](https://github.com/LerianStudio/midaz/commit/29a424ca8f337f67318d8cd17b8df6c20ba36f33))
* added sub command delete in commmand account with test unit ([4a1b77b](https://github.com/LerianStudio/midaz/commit/4a1b77bc3e3b8d2d393793fe8d852ee0e78b41a7))
* added sub command describe in commmand account with test unit ([7990908](https://github.com/LerianStudio/midaz/commit/7990908dde50a023b4a83bd79e159745eb831533))
* added sub command list in commmand account with test unit ([c6d112a](https://github.com/LerianStudio/midaz/commit/c6d112a3d841fb0574479dfb11f1ed8a4e500379))
* added sub command update in commmand account with test unit ([59ba185](https://github.com/LerianStudio/midaz/commit/59ba185856661c0afe3243b88ed68f66b46a4938))
* adjust small issues from swagger docs :sparkles: ([dbdfcf5](https://github.com/LerianStudio/midaz/commit/dbdfcf548aa2bef479ff2fc528506ef66a10da52))
* create git action to update version on env files :sparkles: ([ca28ded](https://github.com/LerianStudio/midaz/commit/ca28ded27672e153adcdbf53db5e2865bd33b123))
* create redis connection :sparkles: ([c8651e5](https://github.com/LerianStudio/midaz/commit/c8651e5c523d2f124dbfa8eaaa3f6647a0d0a5a0))
* create rest get product ([bf9a271](https://github.com/LerianStudio/midaz/commit/bf9a271ddd396e7800c2d69a1f3d87fc00916077))
* create sub command delete from products ([80d3a62](https://github.com/LerianStudio/midaz/commit/80d3a625fe2f02069b1d9e037f4c28bcc2861ccc))
* create sub command update from products ([4368bc2](https://github.com/LerianStudio/midaz/commit/4368bc212f7c4602dad0584feccf903a9e6c2c65))
* implements redis on ledger :sparkles: ([5f1c5e4](https://github.com/LerianStudio/midaz/commit/5f1c5e47aa8507d138ff4739eb966a6beb996212))
* implements redis on transaction :sparkles: ([7013ca2](https://github.com/LerianStudio/midaz/commit/7013ca20499db2b1063890509afbdffd934def97))
* method of creating account rest ([cb4f377](https://github.com/LerianStudio/midaz/commit/cb4f377c047a7a07e64db4ad826691d6198b5f3c))
* method of get by id accounts rest ([b5d61b8](https://github.com/LerianStudio/midaz/commit/b5d61b81deb1384dfaff2d78ec727580b78099d5))
* method of list accounts rest ([5edbc02](https://github.com/LerianStudio/midaz/commit/5edbc027a5df6b61779cd677a98d4dfabafb59fe))
* method of update and delete accounts rest ([551506e](https://github.com/LerianStudio/midaz/commit/551506eb62dce2e38bf8303a23d1e6e8eec887ff))
* rollback lint :sparkles: ([4672464](https://github.com/LerianStudio/midaz/commit/4672464c97531f7817df66d6941d8d535ab45f31))
* test rewiewdog lint :sparkles: ([5d69cc1](https://github.com/LerianStudio/midaz/commit/5d69cc14acbf4658ed832e2ad9ad0dd38ed69018))
* update architecture final stage :sparkles: ([fcd6d6b](https://github.com/LerianStudio/midaz/commit/fcd6d6b4eef2678f21be5dac0d9a1a811a3b3890))
* update git actions :sparkles: ([525b0ac](https://github.com/LerianStudio/midaz/commit/525b0acfc002bacfcc39bd6e3b65a10e9f995377))
* update swagger documentation base using envs and generate docs in dockerfile :sparkles: ([7597ac2](https://github.com/LerianStudio/midaz/commit/7597ac2e46f5731f3e52be46ed0252720ade8021))


### Bug Fixes

* add doc endpoint comment in transaction routes.go ([41f637d](https://github.com/LerianStudio/midaz/commit/41f637d32c37f3e090321d21e46ab0fa180e5e73))
* add logs using default logger in middleware responsible by collecting metrics :bug: :bug: ([d186c0a](https://github.com/LerianStudio/midaz/commit/d186c0afb50fdd3e71e6c80dffc92a6bd25fc30e))
* add required and singletransactiontype tags to transaction input by json endpoint :bug: ([8c4e65f](https://github.com/LerianStudio/midaz/commit/8c4e65f4b2b222a75dba849ec24f2d92d09a400d))
* add validation for scale greater than or equal to zero in transaction by json endpoint :bug: ([c1368a3](https://github.com/LerianStudio/midaz/commit/c1368a33c4aaafba4f366d803665244d00d6f9ce))
* add zap caller skip to ignore hydrated log function :bug: ([03fd066](https://github.com/LerianStudio/midaz/commit/03fd06695dfd1ac68edadbfa50074093c265f976))
* adjust import lint issues :bug: ([9fc524f](https://github.com/LerianStudio/midaz/commit/9fc524f924dc161e8138aaf918d6e10683fc90fb))
* adjust ledger swagger docs :bug: ([1e2c606](https://github.com/LerianStudio/midaz/commit/1e2c606819f154a085a3bd223b4aef1d8b114e19))
* adjust lint issues :bug: ([bce4111](https://github.com/LerianStudio/midaz/commit/bce411179651717a1ead6353fd8a04593f28aafb))
* adjust makefile remove wire. :bug: ([ef13013](https://github.com/LerianStudio/midaz/commit/ef130134c6df8b61b10e174d958bcbd67ccc4fd1))
* adjust to update version once in develop instead of main because rules :bug: ([3f3fdca](https://github.com/LerianStudio/midaz/commit/3f3fdca54493c4a5f4deafa571bb9000f398c597))
* common change to pkg :bug: ([724a9b4](https://github.com/LerianStudio/midaz/commit/724a9b409e8a988c157ced8650c18a446e1e4e74))
* create .keep file to commit folder :bug: ([605c270](https://github.com/LerianStudio/midaz/commit/605c270e7e962cfca1027f149d71b54ffb834601))
* final adjusts :bug: ([c30532f](https://github.com/LerianStudio/midaz/commit/c30532f678b9a1ccc6a1902058279bbdaf90ce14))
* fix merge with two others repos :bug: ([8bb5853](https://github.com/LerianStudio/midaz/commit/8bb5853e63f6254b2a9606a53e070602f3198fd9))
* golint :bug: ([0aae8f8](https://github.com/LerianStudio/midaz/commit/0aae8f8649d288183746fd87cb6669da5161569d))
* include metadata in transaction get all operations endpoint response :bug: ([b07adfa](https://github.com/LerianStudio/midaz/commit/b07adfab0966c7b3c87258806b6615aad273da8b))
* lint :bug: ([1e7f12e](https://github.com/LerianStudio/midaz/commit/1e7f12e82925e9d8f3f10fca6d1f2c13910e8f64))
* lint :bug: ([36b62d4](https://github.com/LerianStudio/midaz/commit/36b62d45a8b2633e9027ccc66e9f1d2c7266d966))
* make lint :bug: ([1a2c76e](https://github.com/LerianStudio/midaz/commit/1a2c76e706b8db611dc76373cf92ee2ec3a2c9c3))
* merge MIDAZ-265 :bug: ([ad73b11](https://github.com/LerianStudio/midaz/commit/ad73b11ec2cef76cbfb7384662f2dbc4fbc74196))
* remove build number from version endpoint in ledger and transaction services :bug: ([798406f](https://github.com/LerianStudio/midaz/commit/798406f2ac00eb9e11fa8076c38906c0aa322f47))
* reorganize imports :bug: ([80a0206](https://github.com/LerianStudio/midaz/commit/80a02066678faec96da5290c1e33adc96eddf89c))
* resolve lint :bug: ([062fe5b](https://github.com/LerianStudio/midaz/commit/062fe5b8acc492c913e31b1039ef8ffbf5a5aff7))
* resolve validation errors in transaction endpoint :bug: ([9203059](https://github.com/LerianStudio/midaz/commit/9203059d4651a1b92de71d3565ab02b27e264d4f))
* rollback version :bug: ([b4543f7](https://github.com/LerianStudio/midaz/commit/b4543f72fcdb9897a6fced1a9314f06fb2edc7d4))
* skip insufficient funds validation for external accounts and update postman collection with new transaction json payload :bug: ([8edcb37](https://github.com/LerianStudio/midaz/commit/8edcb37a6b21b8ddd6b67dda8f2e57b76c82ea0d))
* standardize telemetry and logger shutdown in ledger and transaction services :bug: ([d9246bf](https://github.com/LerianStudio/midaz/commit/d9246bfd85fb5c793b05322d0ed010b8400a15fb))
* types :bug: ([6aed2e1](https://github.com/LerianStudio/midaz/commit/6aed2e1ebc5af1b625351ee643c647cb367cf8ab))
* update :bug: ([981384c](https://github.com/LerianStudio/midaz/commit/981384c9b7f682336db312535b8302883e463b73))
* update comment only instead request changes :bug: ([e3d28eb](https://github.com/LerianStudio/midaz/commit/e3d28eb6b06b045358edc89ca954c0bd0724fa04))
* update erros and imports :bug: ([9e501c4](https://github.com/LerianStudio/midaz/commit/9e501c424aab1fecfbae24a09fc1a50f6ba19f53))
* update git actions name :bug: ([2015cec](https://github.com/LerianStudio/midaz/commit/2015cecdc9b66d2a60ad974ad43e43a4db51a978))
* update imports :bug: ([c0d1d14](https://github.com/LerianStudio/midaz/commit/c0d1d1419ef04ca4340a4f7071841cb587c54ea3))
* update imports names :bug: ([125cfc7](https://github.com/LerianStudio/midaz/commit/125cfc785a831993e478973166f83f84509293a4))
* update ledger makefile to generate swagger docs :bug: ([fe346fd](https://github.com/LerianStudio/midaz/commit/fe346fdfa99892bf29c2e6a0353b1ba8444d0358))
* update make file :bug: ([4847ffd](https://github.com/LerianStudio/midaz/commit/4847ffdb688274cbe65f82200cf93f12f07c0f60))
* update message :bug: ([f39d104](https://github.com/LerianStudio/midaz/commit/f39d1042edbfd00907c7285d3f1c32c753443453))
* update message :bug: ([33269c3](https://github.com/LerianStudio/midaz/commit/33269c3a2dcbdef2b68c7abcdcbfc51e81dbd0a0))
* update transaction error messages to comply with gitbook :bug: ([36ae998](https://github.com/LerianStudio/midaz/commit/36ae9985b908784ea59669087e99cc56e9399f14))
* update transaction value mismatch error message :bug: ([8210e13](https://github.com/LerianStudio/midaz/commit/8210e1303b1838bb5b2f4e174c8f3e7516cc30e7))
* update wire gen with standardize telemetry shutdown in ledger grpc :bug: ([3cf681d](https://github.com/LerianStudio/midaz/commit/3cf681d2ed29f12fdf1606fa250cd94ce33d4109))
* update with lint warning :bug: ([d417fe2](https://github.com/LerianStudio/midaz/commit/d417fe28eae349d3b1b0b2bda1518483576cc31b))
* when both go_version and go_version_file inputs are specified, only go_version will be used :bug: ([62508f8](https://github.com/LerianStudio/midaz/commit/62508f8bd074d8a0b64f66861be3a6101bb36daf))

## [1.29.0-beta.20](https://github.com/LerianStudio/midaz/compare/v1.29.0-beta.19...v1.29.0-beta.20) (2024-11-26)


### Bug Fixes

* adjust to update version once in develop instead of main because rules :bug: ([3f3fdca](https://github.com/LerianStudio/midaz/commit/3f3fdca54493c4a5f4deafa571bb9000f398c597))
* types :bug: ([6aed2e1](https://github.com/LerianStudio/midaz/commit/6aed2e1ebc5af1b625351ee643c647cb367cf8ab))

## [1.29.0-beta.19](https://github.com/LerianStudio/midaz/compare/v1.29.0-beta.18...v1.29.0-beta.19) (2024-11-26)


### Bug Fixes

* adjust import lint issues :bug: ([9fc524f](https://github.com/LerianStudio/midaz/commit/9fc524f924dc161e8138aaf918d6e10683fc90fb))
* include metadata in transaction get all operations endpoint response :bug: ([b07adfa](https://github.com/LerianStudio/midaz/commit/b07adfab0966c7b3c87258806b6615aad273da8b))

## [1.29.0-beta.18](https://github.com/LerianStudio/midaz/compare/v1.29.0-beta.17...v1.29.0-beta.18) (2024-11-26)


### Bug Fixes

* common change to pkg :bug: ([724a9b4](https://github.com/LerianStudio/midaz/commit/724a9b409e8a988c157ced8650c18a446e1e4e74))

## [1.29.0-beta.17](https://github.com/LerianStudio/midaz/compare/v1.29.0-beta.16...v1.29.0-beta.17) (2024-11-26)

## [1.29.0-beta.16](https://github.com/LerianStudio/midaz/compare/v1.29.0-beta.15...v1.29.0-beta.16) (2024-11-26)

## [1.29.0-beta.15](https://github.com/LerianStudio/midaz/compare/v1.29.0-beta.14...v1.29.0-beta.15) (2024-11-26)

## [1.29.0-beta.14](https://github.com/LerianStudio/midaz/compare/v1.29.0-beta.13...v1.29.0-beta.14) (2024-11-26)

## [1.29.0-beta.13](https://github.com/LerianStudio/midaz/compare/v1.29.0-beta.12...v1.29.0-beta.13) (2024-11-26)

## [1.29.0-beta.12](https://github.com/LerianStudio/midaz/compare/v1.29.0-beta.11...v1.29.0-beta.12) (2024-11-25)


### Features

* add swagger documentation to transaction service :sparkles: ([e06a30e](https://github.com/LerianStudio/midaz/commit/e06a30e360e70079ce66c7f3aeecdd5536c8b134))
* add swagger generated docs from transaction :sparkles: ([a6e3775](https://github.com/LerianStudio/midaz/commit/a6e377576673c4a2c0a2691f717518d9ade65e0f))


### Bug Fixes

* adjust ledger swagger docs :bug: ([1e2c606](https://github.com/LerianStudio/midaz/commit/1e2c606819f154a085a3bd223b4aef1d8b114e19))

## [1.29.0-beta.11](https://github.com/LerianStudio/midaz/compare/v1.29.0-beta.10...v1.29.0-beta.11) (2024-11-25)


### Bug Fixes

* create .keep file to commit folder :bug: ([605c270](https://github.com/LerianStudio/midaz/commit/605c270e7e962cfca1027f149d71b54ffb834601))
* final adjusts :bug: ([c30532f](https://github.com/LerianStudio/midaz/commit/c30532f678b9a1ccc6a1902058279bbdaf90ce14))
* rollback version :bug: ([b4543f7](https://github.com/LerianStudio/midaz/commit/b4543f72fcdb9897a6fced1a9314f06fb2edc7d4))

## [1.29.0-beta.10](https://github.com/LerianStudio/midaz/compare/v1.29.0-beta.9...v1.29.0-beta.10) (2024-11-25)


### Bug Fixes

* update ledger makefile to generate swagger docs :bug: ([fe346fd](https://github.com/LerianStudio/midaz/commit/fe346fdfa99892bf29c2e6a0353b1ba8444d0358))

## [1.29.0-beta.9](https://github.com/LerianStudio/midaz/compare/v1.29.0-beta.8...v1.29.0-beta.9) (2024-11-25)


### Features

* add base to the swagger documentation and telemetry root span handling for the swagger endpoint calls :sparkles: ([0165a7c](https://github.com/LerianStudio/midaz/commit/0165a7c996a59e5941a2448e03e461b57088a677))
* add swagger documentation generated for ledger :sparkles: ([cef9e22](https://github.com/LerianStudio/midaz/commit/cef9e22ee6558dc16372ab17e688129a5856212c))
* add swagger documentation to onboarding context on ledger service :sparkles: ([65ea499](https://github.com/LerianStudio/midaz/commit/65ea499a50e17f6e22f52f9705a833e4d64a134a))
* add swagger documentation to the portfolio context on ledger service :sparkles: ([fad4b08](https://github.com/LerianStudio/midaz/commit/fad4b08dbb7a0ee47f5b784ccef668d2843bab4f))
* adjust small issues from swagger docs :sparkles: ([dbdfcf5](https://github.com/LerianStudio/midaz/commit/dbdfcf548aa2bef479ff2fc528506ef66a10da52))
* update architecture final stage :sparkles: ([fcd6d6b](https://github.com/LerianStudio/midaz/commit/fcd6d6b4eef2678f21be5dac0d9a1a811a3b3890))
* update swagger documentation base using envs and generate docs in dockerfile :sparkles: ([7597ac2](https://github.com/LerianStudio/midaz/commit/7597ac2e46f5731f3e52be46ed0252720ade8021))


### Bug Fixes

* adjust lint issues :bug: ([bce4111](https://github.com/LerianStudio/midaz/commit/bce411179651717a1ead6353fd8a04593f28aafb))
* adjust makefile remove wire. :bug: ([ef13013](https://github.com/LerianStudio/midaz/commit/ef130134c6df8b61b10e174d958bcbd67ccc4fd1))
* fix merge with two others repos :bug: ([8bb5853](https://github.com/LerianStudio/midaz/commit/8bb5853e63f6254b2a9606a53e070602f3198fd9))
* lint :bug: ([36b62d4](https://github.com/LerianStudio/midaz/commit/36b62d45a8b2633e9027ccc66e9f1d2c7266d966))
* make lint :bug: ([1a2c76e](https://github.com/LerianStudio/midaz/commit/1a2c76e706b8db611dc76373cf92ee2ec3a2c9c3))
* merge MIDAZ-265 :bug: ([ad73b11](https://github.com/LerianStudio/midaz/commit/ad73b11ec2cef76cbfb7384662f2dbc4fbc74196))
* reorganize imports :bug: ([80a0206](https://github.com/LerianStudio/midaz/commit/80a02066678faec96da5290c1e33adc96eddf89c))
* standardize telemetry and logger shutdown in ledger and transaction services :bug: ([d9246bf](https://github.com/LerianStudio/midaz/commit/d9246bfd85fb5c793b05322d0ed010b8400a15fb))
* update erros and imports :bug: ([9e501c4](https://github.com/LerianStudio/midaz/commit/9e501c424aab1fecfbae24a09fc1a50f6ba19f53))
* update imports :bug: ([c0d1d14](https://github.com/LerianStudio/midaz/commit/c0d1d1419ef04ca4340a4f7071841cb587c54ea3))
* update imports names :bug: ([125cfc7](https://github.com/LerianStudio/midaz/commit/125cfc785a831993e478973166f83f84509293a4))
* update make file :bug: ([4847ffd](https://github.com/LerianStudio/midaz/commit/4847ffdb688274cbe65f82200cf93f12f07c0f60))
* update wire gen with standardize telemetry shutdown in ledger grpc :bug: ([3cf681d](https://github.com/LerianStudio/midaz/commit/3cf681d2ed29f12fdf1606fa250cd94ce33d4109))
* update with lint warning :bug: ([d417fe2](https://github.com/LerianStudio/midaz/commit/d417fe28eae349d3b1b0b2bda1518483576cc31b))

## [1.29.0-beta.8](https://github.com/LerianStudio/midaz/compare/v1.29.0-beta.7...v1.29.0-beta.8) (2024-11-21)


### Bug Fixes

* add logs using default logger in middleware responsible by collecting metrics :bug: :bug: ([d186c0a](https://github.com/LerianStudio/midaz/commit/d186c0afb50fdd3e71e6c80dffc92a6bd25fc30e))
* add required and singletransactiontype tags to transaction input by json endpoint :bug: ([8c4e65f](https://github.com/LerianStudio/midaz/commit/8c4e65f4b2b222a75dba849ec24f2d92d09a400d))
* add validation for scale greater than or equal to zero in transaction by json endpoint :bug: ([c1368a3](https://github.com/LerianStudio/midaz/commit/c1368a33c4aaafba4f366d803665244d00d6f9ce))
* add zap caller skip to ignore hydrated log function :bug: ([03fd066](https://github.com/LerianStudio/midaz/commit/03fd06695dfd1ac68edadbfa50074093c265f976))
* resolve validation errors in transaction endpoint :bug: ([9203059](https://github.com/LerianStudio/midaz/commit/9203059d4651a1b92de71d3565ab02b27e264d4f))
* skip insufficient funds validation for external accounts and update postman collection with new transaction json payload :bug: ([8edcb37](https://github.com/LerianStudio/midaz/commit/8edcb37a6b21b8ddd6b67dda8f2e57b76c82ea0d))
* update transaction value mismatch error message :bug: ([8210e13](https://github.com/LerianStudio/midaz/commit/8210e1303b1838bb5b2f4e174c8f3e7516cc30e7))

## [1.29.0-beta.7](https://github.com/LerianStudio/midaz/compare/v1.29.0-beta.6...v1.29.0-beta.7) (2024-11-21)


### Features

* added command account in root ([7e2a439](https://github.com/LerianStudio/midaz/commit/7e2a439a26efa5786a5352b09875339d7545b2e6))
* added sub command create in commmand account with test unit ([29a424c](https://github.com/LerianStudio/midaz/commit/29a424ca8f337f67318d8cd17b8df6c20ba36f33))
* added sub command delete in commmand account with test unit ([4a1b77b](https://github.com/LerianStudio/midaz/commit/4a1b77bc3e3b8d2d393793fe8d852ee0e78b41a7))
* added sub command describe in commmand account with test unit ([7990908](https://github.com/LerianStudio/midaz/commit/7990908dde50a023b4a83bd79e159745eb831533))
* added sub command list in commmand account with test unit ([c6d112a](https://github.com/LerianStudio/midaz/commit/c6d112a3d841fb0574479dfb11f1ed8a4e500379))
* added sub command update in commmand account with test unit ([59ba185](https://github.com/LerianStudio/midaz/commit/59ba185856661c0afe3243b88ed68f66b46a4938))
* method of creating account rest ([cb4f377](https://github.com/LerianStudio/midaz/commit/cb4f377c047a7a07e64db4ad826691d6198b5f3c))
* method of get by id accounts rest ([b5d61b8](https://github.com/LerianStudio/midaz/commit/b5d61b81deb1384dfaff2d78ec727580b78099d5))
* method of list accounts rest ([5edbc02](https://github.com/LerianStudio/midaz/commit/5edbc027a5df6b61779cd677a98d4dfabafb59fe))
* method of update and delete accounts rest ([551506e](https://github.com/LerianStudio/midaz/commit/551506eb62dce2e38bf8303a23d1e6e8eec887ff))

## [1.29.0-beta.6](https://github.com/LerianStudio/midaz/compare/v1.29.0-beta.5...v1.29.0-beta.6) (2024-11-19)


### Features

* create git action to update version on env files :sparkles: ([ca28ded](https://github.com/LerianStudio/midaz/commit/ca28ded27672e153adcdbf53db5e2865bd33b123))

## [1.29.0-beta.5](https://github.com/LerianStudio/midaz/compare/v1.29.0-beta.4...v1.29.0-beta.5) (2024-11-18)


### Features

* added command describe from products ([4b4a222](https://github.com/LerianStudio/midaz/commit/4b4a22273e009760e2819b04063a8715388fdfa1))
* create redis connection :sparkles: ([c8651e5](https://github.com/LerianStudio/midaz/commit/c8651e5c523d2f124dbfa8eaaa3f6647a0d0a5a0))
* create sub command delete from products ([80d3a62](https://github.com/LerianStudio/midaz/commit/80d3a625fe2f02069b1d9e037f4c28bcc2861ccc))
* create sub command update from products ([4368bc2](https://github.com/LerianStudio/midaz/commit/4368bc212f7c4602dad0584feccf903a9e6c2c65))
* implements redis on ledger :sparkles: ([5f1c5e4](https://github.com/LerianStudio/midaz/commit/5f1c5e47aa8507d138ff4739eb966a6beb996212))
* implements redis on transaction :sparkles: ([7013ca2](https://github.com/LerianStudio/midaz/commit/7013ca20499db2b1063890509afbdffd934def97))


### Bug Fixes

* lint :bug: ([1e7f12e](https://github.com/LerianStudio/midaz/commit/1e7f12e82925e9d8f3f10fca6d1f2c13910e8f64))

## [1.29.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.29.0-beta.3...v1.29.0-beta.4) (2024-11-18)


### Features

* add version endpoint to ledger and transaction services :sparkles: ([bb646b7](https://github.com/LerianStudio/midaz/commit/bb646b75161b1698adacc32164862d910fa5e987))


### Bug Fixes

* add doc endpoint comment in transaction routes.go ([41f637d](https://github.com/LerianStudio/midaz/commit/41f637d32c37f3e090321d21e46ab0fa180e5e73))
* remove build number from version endpoint in ledger and transaction services :bug: ([798406f](https://github.com/LerianStudio/midaz/commit/798406f2ac00eb9e11fa8076c38906c0aa322f47))

## [1.29.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.29.0-beta.2...v1.29.0-beta.3) (2024-11-18)


### Bug Fixes

* update transaction error messages to comply with gitbook :bug: ([36ae998](https://github.com/LerianStudio/midaz/commit/36ae9985b908784ea59669087e99cc56e9399f14))

## [1.29.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.29.0-beta.1...v1.29.0-beta.2) (2024-11-18)


### Features

* added command list from products ([fe7503e](https://github.com/LerianStudio/midaz/commit/fe7503ea6c4b971be4ffba55ed21035bfeb15710))
* create rest get product ([bf9a271](https://github.com/LerianStudio/midaz/commit/bf9a271ddd396e7800c2d69a1f3d87fc00916077))

## [1.29.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.28.0...v1.29.0-beta.1) (2024-11-14)


### Features

* add :sparkles: ([8baab22](https://github.com/LerianStudio/midaz/commit/8baab221b425c84fc56ee1eadcb8da3d09048543))
* add blocked to open pr to main if not come from develop or hotfix :sparkles: ([327448d](https://github.com/LerianStudio/midaz/commit/327448dafbd03db064c0f9488c0950e270d6556f))
* add reviewdog :sparkles: ([e5af335](https://github.com/LerianStudio/midaz/commit/e5af335e030c4e1ee7c68ec7ba6997db7c56cd4c))
* add reviewdog again :sparkles: ([3636404](https://github.com/LerianStudio/midaz/commit/363640416c1c263238ab8e3634f90cef348b8c5e))
* add rule to pr :sparkles: ([6e0ff0c](https://github.com/LerianStudio/midaz/commit/6e0ff0c010ea23feb1e3140ebe8e88abca2ae547))
* rollback lint :sparkles: ([4672464](https://github.com/LerianStudio/midaz/commit/4672464c97531f7817df66d6941d8d535ab45f31))
* test rewiewdog lint :sparkles: ([5d69cc1](https://github.com/LerianStudio/midaz/commit/5d69cc14acbf4658ed832e2ad9ad0dd38ed69018))
* update git actions :sparkles: ([525b0ac](https://github.com/LerianStudio/midaz/commit/525b0acfc002bacfcc39bd6e3b65a10e9f995377))


### Bug Fixes

* golint :bug: ([0aae8f8](https://github.com/LerianStudio/midaz/commit/0aae8f8649d288183746fd87cb6669da5161569d))
* resolve lint :bug: ([062fe5b](https://github.com/LerianStudio/midaz/commit/062fe5b8acc492c913e31b1039ef8ffbf5a5aff7))
* update :bug: ([981384c](https://github.com/LerianStudio/midaz/commit/981384c9b7f682336db312535b8302883e463b73))
* update comment only instead request changes :bug: ([e3d28eb](https://github.com/LerianStudio/midaz/commit/e3d28eb6b06b045358edc89ca954c0bd0724fa04))
* update git actions name :bug: ([2015cec](https://github.com/LerianStudio/midaz/commit/2015cecdc9b66d2a60ad974ad43e43a4db51a978))
* update message :bug: ([f39d104](https://github.com/LerianStudio/midaz/commit/f39d1042edbfd00907c7285d3f1c32c753443453))
* update message :bug: ([33269c3](https://github.com/LerianStudio/midaz/commit/33269c3a2dcbdef2b68c7abcdcbfc51e81dbd0a0))
* when both go_version and go_version_file inputs are specified, only go_version will be used :bug: ([62508f8](https://github.com/LerianStudio/midaz/commit/62508f8bd074d8a0b64f66861be3a6101bb36daf))

## [1.28.0](https://github.com/LerianStudio/midaz/compare/v1.27.0...v1.28.0) (2024-11-14)


### Features

* added command product in root ([d0c2f89](https://github.com/LerianStudio/midaz/commit/d0c2f898e2ad29fc864eb3545b0cd0eb86caaec3))
* added new sub command create on command product ([9c63088](https://github.com/LerianStudio/midaz/commit/9c63088ffa88747e95a7254f49d8d00c180e1434))

## [1.27.0](https://github.com/LerianStudio/midaz/compare/v1.26.1...v1.27.0) (2024-11-13)


### Features

* add definitions and config :sparkles: ([a49b010](https://github.com/LerianStudio/midaz/commit/a49b010269122600bdf6ed0fa02a5b6aa9f703d4))
* add grafana-lgtm and open telemetry collector to infra docker-compose :sparkles: ([6351d3b](https://github.com/LerianStudio/midaz/commit/6351d3bc5db24ac09afa693909ee2725c2a5b012))
* add opentelemetry traces to account endpoints :sparkles: ([bf7f043](https://github.com/LerianStudio/midaz/commit/bf7f04303d36e15a61af5fb1dde1476e658e5029))
* add opentelemetry traces to account endpoints and abstract context functions in common package :sparkles: ([c5861e7](https://github.com/LerianStudio/midaz/commit/c5861e733ec390f9da92f53d221347ecc3046701))
* add opentelemetry traces to asset endpoints :sparkles: ([3eb7f9a](https://github.com/LerianStudio/midaz/commit/3eb7f9a34e166fc7a0d798f49ac4ccfb5dc62b8a))
* add opentelemetry traces to operation endpoints and update business error responses :sparkles: ([b6568b8](https://github.com/LerianStudio/midaz/commit/b6568b8369c8ebca79bbc19266981353026da545))
* add opentelemetry traces to portfolio endpoints :sparkles: ([cc442f8](https://github.com/LerianStudio/midaz/commit/cc442f85568e7717de706c73a9515400a4bfa651))
* add opentelemetry traces to products endpoints :sparkles: ([2f3e78a](https://github.com/LerianStudio/midaz/commit/2f3e78a7d2f4ef71fc29abc51b9183d5685f568b))
* add opentelemetry traces to transaction endpoints :sparkles: ([442c71f](https://github.com/LerianStudio/midaz/commit/442c71f0a06182d7adfdf2579d50247e3500d863))
* add opentelemetry tracing propagation to transaction and ledger endpoints :sparkles: ([19d8e51](https://github.com/LerianStudio/midaz/commit/19d8e518e367a993051974ff1a2174e9bfaa3d57))
* add repository on command and query :sparkles: ([94d254a](https://github.com/LerianStudio/midaz/commit/94d254ae9e74ce7ac9509e625228eba019b4e7a1))
* add traces to the ledger endpoints using opentelemetry :sparkles: ([4c7944b](https://github.com/LerianStudio/midaz/commit/4c7944baeb13f1a410960437b9306feb9c581f44))
* add traces to the organization endpoints using opentelemetry :sparkles: ([cc3c62f](https://github.com/LerianStudio/midaz/commit/cc3c62f03688f6847122d6cb65dec8703d86b0b5))
* add tracing telemetry to create organization endpoint :sparkles: ([b1b2f11](https://github.com/LerianStudio/midaz/commit/b1b2f115607b34777a1024226544f5c0e017b083))
* added new sub command create on command portfolio ([5692c79](https://github.com/LerianStudio/midaz/commit/5692c791119335da27657b91eaca1933401669d0))
* added new sub command delete on command asset ([d7a91f4](https://github.com/LerianStudio/midaz/commit/d7a91f44198d519e6d122d8091a137c37cbdd708))
* added new sub command delete on command portfolio ([ee48586](https://github.com/LerianStudio/midaz/commit/ee48586a91e40e2ffe1983bacb36c1bc6ef56c6d))
* added new sub command describe on command asset ([5d14dab](https://github.com/LerianStudio/midaz/commit/5d14dabe4a67a3e97f2cd52fa33f50b27bec782a))
* added new sub command describe on command portfolio ([0d3b154](https://github.com/LerianStudio/midaz/commit/0d3b15451d48b234d72e726fd09f5116706b6c34))
* added new sub command list on command portfolio ([d652feb](https://github.com/LerianStudio/midaz/commit/d652feb3e175835dd2590cf2942abf58d5dcd18b))
* added new sub command list on command portfolio ([11f6f07](https://github.com/LerianStudio/midaz/commit/11f6f079c70bab16058747bc7e6fca34a10a132c))
* added new sub command update on command asset ([2edf239](https://github.com/LerianStudio/midaz/commit/2edf2397b13dbbc114937ad3e20192b34931c5a7))
* added new sub command update on command portfolio ([87e9977](https://github.com/LerianStudio/midaz/commit/87e99770563db5dd6afe37326e27c0e6b0b63816))
* adjust wire inject :sparkles: ([ca0ddb4](https://github.com/LerianStudio/midaz/commit/ca0ddb40cd490353126108fe36334241f7cb714c))
* **transaction:** create connection files and add amqp on go.mod :sparkles: ([63f816f](https://github.com/LerianStudio/midaz/commit/63f816fcc7d64b570b8495a1ee338e6891ee520a))
* create mocks based on repositories :sparkles: ([f737239](https://github.com/LerianStudio/midaz/commit/f737239876cf9ad944289e4d3ea1491bf37003dd))
* create producer and consumer repositories :sparkles: ([474d2d0](https://github.com/LerianStudio/midaz/commit/474d2d052a32930b75e4abb3cd1be6dc04da1092))
* create rabbitmq handler on ports/http :sparkles: ([96b6b23](https://github.com/LerianStudio/midaz/commit/96b6b23c9a0e6e8b31ea3329f3b7082a0ecdcb93))
* create rest get by id asset ([059d6a1](https://github.com/LerianStudio/midaz/commit/059d6a187a9c4ef2905249e5dc60527451c3fbec))
* create rest get by id portfolio ([97db29c](https://github.com/LerianStudio/midaz/commit/97db29c26661494c78809ad6109cee5907109c9c))
* create rest update ledger ([b2f8129](https://github.com/LerianStudio/midaz/commit/b2f81295f8773a4d5b4c26e7d306122d1c2f1ee8))
* create sub command delete from command ledger with test unit of the command delete ([63de66e](https://github.com/LerianStudio/midaz/commit/63de66eff8e604e13bae20d3842c4c6302f93503))
* create sub command update from command ledger ([57fc305](https://github.com/LerianStudio/midaz/commit/57fc305d5bfd7cd6eaab25c651b27c3bb604a02b))
* create test to producer and consumer; :sparkles: ([929d825](https://github.com/LerianStudio/midaz/commit/929d825ba8749aab4520e4dac7a8109125f27952))
* created asset command, creation of the create subcommand of the asset command ([bdace84](https://github.com/LerianStudio/midaz/commit/bdace84be5e1d8909439a5d91d67cf86e16d6e90))
* created subcommand list of the command asset ([c2d19fc](https://github.com/LerianStudio/midaz/commit/c2d19fc6bdb29b64f9f5435dd8dcb5d23115ad04))
* implement handler; :sparkles: ([dc9df25](https://github.com/LerianStudio/midaz/commit/dc9df25a6c770a7f361094bd835ae042ad5a1aec))
* implement on cqrs; :sparkles: ([d122ba6](https://github.com/LerianStudio/midaz/commit/d122ba63d7652188622a7a6795616b19fdc86153))
* implement on routes; :sparkles: ([db9b12f](https://github.com/LerianStudio/midaz/commit/db9b12fc69f37e63e7b2006638acd439d3f51035))
* implement producer and consumer on adapters :sparkles: ([4ff04d4](https://github.com/LerianStudio/midaz/commit/4ff04d4295d7adf12f96815070c2f987bb6cc231))
* implement rabbitmq config, inject and wire; :sparkles: ([5baae29](https://github.com/LerianStudio/midaz/commit/5baae29c08e680c65419c7457ec1adc1ce6d4f9a))
* implement rabbitmq on ledger :sparkles: ([17a9c3d](https://github.com/LerianStudio/midaz/commit/17a9c3da33d2c6a8c9720b7d5d7c550a98b35a04))
* implementation mock :sparkles: ([481e856](https://github.com/LerianStudio/midaz/commit/481e856bf004dc4539c0105cba7bd3d05859c1e5))
* implements producer and consumer with interface and implementation :sparkles: ([5dccc86](https://github.com/LerianStudio/midaz/commit/5dccc86eeb1847dc8dc99d835b6a0fed5888b043))
* init of implementing rabbitmq :sparkles: ([ba9dc6f](https://github.com/LerianStudio/midaz/commit/ba9dc6f567d592ba6628ea61273d970e867b53e6))
* method delete rest api ledger ([e8917de](https://github.com/LerianStudio/midaz/commit/e8917ded93e7fb3d9bbaa38e66c5734e1fe8b41b))


### Bug Fixes

* add comments :bug: ([5c1bbf7](https://github.com/LerianStudio/midaz/commit/5c1bbf7df3be2fc1171c06ac720bc035535331ff))
* add opentelemetry traces to asset rate endpoints and small adjusts to ledger metadata tracing and wire inject file :bug: ([d933b13](https://github.com/LerianStudio/midaz/commit/d933b13db0b539ba19471af40c808e669baade93))
* add span error setting on withTelemetry file :bug: ([40a2008](https://github.com/LerianStudio/midaz/commit/40a20089658b8cc58d52be224a1478c55623a693))
* adjust data type in transaction from json endpoint in postman collection :bug: ([107b60f](https://github.com/LerianStudio/midaz/commit/107b60f980ae2138e497216f95032ba2200a5858))
* adjust lint ineffectual assignment to ctx :bug: ([e78cef5](https://github.com/LerianStudio/midaz/commit/e78cef569a78206b6859c0eb4ad51486fa8c72a3))
* ah metadata structure is totally optional now, it caused errors when trying to request with null fields in the api ([3dac45f](https://github.com/LerianStudio/midaz/commit/3dac45fea9bd1c2fef7990289d4c33eb5884d182))
* complete conn and health on rabbitmq :bug: ([61d1431](https://github.com/LerianStudio/midaz/commit/61d143170704a8cb35b33395e61812fc31f206f5))
* create new users to separate ledger and transaction :bug: ([24f66c8](https://github.com/LerianStudio/midaz/commit/24f66c8bb43938a5e44206853153542b51a9471c))
* login via flag no save token local ([656b15a](https://github.com/LerianStudio/midaz/commit/656b15a964a22eb400fae1716b7c10c649283265))
* make lint :bug: ([5a7847a](https://github.com/LerianStudio/midaz/commit/5a7847aea01f89d606604c4311e4539347ba26f3))
* make lint; :bug: ([3e55c43](https://github.com/LerianStudio/midaz/commit/3e55c436db91bb34c2f61a3671313eaf449988a9))
* mock :bug: ([5b2d152](https://github.com/LerianStudio/midaz/commit/5b2d152ff987638a36bc643796aa9d755b0e53fc))
* move opentelemetry init to before logger init and move logger provider initialization to otel common file :bug: ([a25af7f](https://github.com/LerianStudio/midaz/commit/a25af7f78c02159b4f39a5eeb9e66675467c617b))
* remove line break from generate uuidv7 func :bug: ([7cf4009](https://github.com/LerianStudio/midaz/commit/7cf4009e9dbb984d3aa94e4c3132645f0c99ca0b))
* remove otel-collector from infra module and keep otel-lgtm as the final opentelemetry backend :bug: ([07df708](https://github.com/LerianStudio/midaz/commit/07df7088da9ea48771d562716e5524f451de9848))
* remove producer and consumer from commons :bug: ([fec19a9](https://github.com/LerianStudio/midaz/commit/fec19a901d4af1ec05eaf488ab721c501d2b9714))
* remove short telemetry upload interval used for development purposes :bug: ([64481fb](https://github.com/LerianStudio/midaz/commit/64481fb8fb9c9ea4c8ddc1f3d4b1e66134154782))
* **transaction:** remove test handler :bug: ([8081dcf](https://github.com/LerianStudio/midaz/commit/8081dcf69a654132062a8915cff02b0213765d03))
* **ledger:** remove test handler; :bug: ([2dc3803](https://github.com/LerianStudio/midaz/commit/2dc38035c3db16ea905fd5955eb99788af8edb70))
* remove unusued alias common :bug: ([cdd77f1](https://github.com/LerianStudio/midaz/commit/cdd77f1681cc8a5ef90a8aaf62592ab7aec91b76))
* uncomment grafana-lgtm and otel-collector on infra docker-compose :bug: ([07dabfd](https://github.com/LerianStudio/midaz/commit/07dabfd64fa09773e04253aa90ca54e90b356623))
* update amqp :bug: ([b2d6d22](https://github.com/LerianStudio/midaz/commit/b2d6d22d48251c6b377b503dbf848b07e7c09fc9))
* update bug :bug: ([fdbe8ed](https://github.com/LerianStudio/midaz/commit/fdbe8ed16808ced0782f8d55a5d4d6cd16c9140c))
* update imports that refactor missed :bug: ([e6e9502](https://github.com/LerianStudio/midaz/commit/e6e95020272edb77ea38cb3fc80b4fc0b901d8b3))
* update infra docker compose to use envs on otel containers and in his yaml config file :bug: ([a6ba7cb](https://github.com/LerianStudio/midaz/commit/a6ba7cbfc07baa095d785c6283c0048243333078))
* update infra otel containers to comply with midaz container name pattern :bug: ([7c067d4](https://github.com/LerianStudio/midaz/commit/7c067d40a23d7543bbae22678d6ce232fdcc1bd4))
* update injection; :bug: ([2da2b58](https://github.com/LerianStudio/midaz/commit/2da2b5808844b9f248c9d557647e5851d815393d))
* update make lint :bug: ([0945d37](https://github.com/LerianStudio/midaz/commit/0945d37a3ab5ce534ab4de6bda463441751dab2a))
* update postman midaz id header to comply with id uppercase pattern :bug: ([a219509](https://github.com/LerianStudio/midaz/commit/a219509bcbd4a147dc82dadc577b200c1fd8147b))
* update trace error treatment in ledger find by name repository func :bug: ([cfd86a4](https://github.com/LerianStudio/midaz/commit/cfd86a43f32562482ecf8a0e4822804b46ebf4cc))

## [1.27.0-beta.28](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.27...v1.27.0-beta.28) (2024-11-13)


### Bug Fixes

* remove line break from generate uuidv7 func :bug: ([7cf4009](https://github.com/LerianStudio/midaz/commit/7cf4009e9dbb984d3aa94e4c3132645f0c99ca0b))

## [1.27.0-beta.27](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.26...v1.27.0-beta.27) (2024-11-13)


### Bug Fixes

* add span error setting on withTelemetry file :bug: ([40a2008](https://github.com/LerianStudio/midaz/commit/40a20089658b8cc58d52be224a1478c55623a693))
* update postman midaz id header to comply with id uppercase pattern :bug: ([a219509](https://github.com/LerianStudio/midaz/commit/a219509bcbd4a147dc82dadc577b200c1fd8147b))

## [1.27.0-beta.26](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.25...v1.27.0-beta.26) (2024-11-13)


### Bug Fixes

* add comments :bug: ([5c1bbf7](https://github.com/LerianStudio/midaz/commit/5c1bbf7df3be2fc1171c06ac720bc035535331ff))
* update imports that refactor missed :bug: ([e6e9502](https://github.com/LerianStudio/midaz/commit/e6e95020272edb77ea38cb3fc80b4fc0b901d8b3))

## [1.27.0-beta.25](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.24...v1.27.0-beta.25) (2024-11-13)


### Features

* added new sub command delete on command portfolio ([ee48586](https://github.com/LerianStudio/midaz/commit/ee48586a91e40e2ffe1983bacb36c1bc6ef56c6d))

## [1.27.0-beta.24](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.23...v1.27.0-beta.24) (2024-11-13)


### Features

* added new sub command update on command portfolio ([87e9977](https://github.com/LerianStudio/midaz/commit/87e99770563db5dd6afe37326e27c0e6b0b63816))

## [1.27.0-beta.23](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.22...v1.27.0-beta.23) (2024-11-13)


### Bug Fixes

* remove otel-collector from infra module and keep otel-lgtm as the final opentelemetry backend :bug: ([07df708](https://github.com/LerianStudio/midaz/commit/07df7088da9ea48771d562716e5524f451de9848))

## [1.27.0-beta.22](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.21...v1.27.0-beta.22) (2024-11-13)


### Features

* added new sub command describe on command portfolio ([0d3b154](https://github.com/LerianStudio/midaz/commit/0d3b15451d48b234d72e726fd09f5116706b6c34))
* create rest get by id portfolio ([97db29c](https://github.com/LerianStudio/midaz/commit/97db29c26661494c78809ad6109cee5907109c9c))

## [1.27.0-beta.21](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.20...v1.27.0-beta.21) (2024-11-12)


### Features

* added new sub command list on command portfolio ([d652feb](https://github.com/LerianStudio/midaz/commit/d652feb3e175835dd2590cf2942abf58d5dcd18b))
* added new sub command list on command portfolio ([11f6f07](https://github.com/LerianStudio/midaz/commit/11f6f079c70bab16058747bc7e6fca34a10a132c))

## [1.27.0-beta.20](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.19...v1.27.0-beta.20) (2024-11-12)


### Features

* added new sub command create on command portfolio ([5692c79](https://github.com/LerianStudio/midaz/commit/5692c791119335da27657b91eaca1933401669d0))

## [1.27.0-beta.19](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.18...v1.27.0-beta.19) (2024-11-12)


### Features

* add opentelemetry traces to operation endpoints and update business error responses :sparkles: ([b6568b8](https://github.com/LerianStudio/midaz/commit/b6568b8369c8ebca79bbc19266981353026da545))
* add opentelemetry traces to transaction endpoints :sparkles: ([442c71f](https://github.com/LerianStudio/midaz/commit/442c71f0a06182d7adfdf2579d50247e3500d863))
* add opentelemetry tracing propagation to transaction and ledger endpoints :sparkles: ([19d8e51](https://github.com/LerianStudio/midaz/commit/19d8e518e367a993051974ff1a2174e9bfaa3d57))


### Bug Fixes

* adjust data type in transaction from json endpoint in postman collection :bug: ([107b60f](https://github.com/LerianStudio/midaz/commit/107b60f980ae2138e497216f95032ba2200a5858))

## [1.27.0-beta.18](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.17...v1.27.0-beta.18) (2024-11-12)

## [1.27.0-beta.17](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.16...v1.27.0-beta.17) (2024-11-12)

## [1.27.0-beta.16](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.15...v1.27.0-beta.16) (2024-11-12)

## [1.27.0-beta.15](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.14...v1.27.0-beta.15) (2024-11-12)

## [1.27.0-beta.14](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.13...v1.27.0-beta.14) (2024-11-12)

## [1.27.0-beta.13](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.12...v1.27.0-beta.13) (2024-11-11)


### Bug Fixes

* update bug :bug: ([fdbe8ed](https://github.com/LerianStudio/midaz/commit/fdbe8ed16808ced0782f8d55a5d4d6cd16c9140c))
* update make lint :bug: ([0945d37](https://github.com/LerianStudio/midaz/commit/0945d37a3ab5ce534ab4de6bda463441751dab2a))

## [1.27.0-beta.12](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.11...v1.27.0-beta.12) (2024-11-11)


### Bug Fixes

* add opentelemetry traces to asset rate endpoints and small adjusts to ledger metadata tracing and wire inject file :bug: ([d933b13](https://github.com/LerianStudio/midaz/commit/d933b13db0b539ba19471af40c808e669baade93))
* adjust lint ineffectual assignment to ctx :bug: ([e78cef5](https://github.com/LerianStudio/midaz/commit/e78cef569a78206b6859c0eb4ad51486fa8c72a3))
* move opentelemetry init to before logger init and move logger provider initialization to otel common file :bug: ([a25af7f](https://github.com/LerianStudio/midaz/commit/a25af7f78c02159b4f39a5eeb9e66675467c617b))

## [1.27.0-beta.11](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.10...v1.27.0-beta.11) (2024-11-11)


### Features

* added new sub command delete on command asset ([d7a91f4](https://github.com/LerianStudio/midaz/commit/d7a91f44198d519e6d122d8091a137c37cbdd708))

## [1.27.0-beta.10](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.9...v1.27.0-beta.10) (2024-11-11)


### Features

* add opentelemetry traces to account endpoints :sparkles: ([bf7f043](https://github.com/LerianStudio/midaz/commit/bf7f04303d36e15a61af5fb1dde1476e658e5029))
* add opentelemetry traces to account endpoints and abstract context functions in common package :sparkles: ([c5861e7](https://github.com/LerianStudio/midaz/commit/c5861e733ec390f9da92f53d221347ecc3046701))
* add opentelemetry traces to asset endpoints :sparkles: ([3eb7f9a](https://github.com/LerianStudio/midaz/commit/3eb7f9a34e166fc7a0d798f49ac4ccfb5dc62b8a))
* add opentelemetry traces to portfolio endpoints :sparkles: ([cc442f8](https://github.com/LerianStudio/midaz/commit/cc442f85568e7717de706c73a9515400a4bfa651))
* add opentelemetry traces to products endpoints :sparkles: ([2f3e78a](https://github.com/LerianStudio/midaz/commit/2f3e78a7d2f4ef71fc29abc51b9183d5685f568b))


### Bug Fixes

* remove short telemetry upload interval used for development purposes :bug: ([64481fb](https://github.com/LerianStudio/midaz/commit/64481fb8fb9c9ea4c8ddc1f3d4b1e66134154782))
* update infra otel containers to comply with midaz container name pattern :bug: ([7c067d4](https://github.com/LerianStudio/midaz/commit/7c067d40a23d7543bbae22678d6ce232fdcc1bd4))
* update trace error treatment in ledger find by name repository func :bug: ([cfd86a4](https://github.com/LerianStudio/midaz/commit/cfd86a43f32562482ecf8a0e4822804b46ebf4cc))

## [1.27.0-beta.9](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.8...v1.27.0-beta.9) (2024-11-08)


### Features

* add traces to the ledger endpoints using opentelemetry :sparkles: ([4c7944b](https://github.com/LerianStudio/midaz/commit/4c7944baeb13f1a410960437b9306feb9c581f44))
* add traces to the organization endpoints using opentelemetry :sparkles: ([cc3c62f](https://github.com/LerianStudio/midaz/commit/cc3c62f03688f6847122d6cb65dec8703d86b0b5))
* add tracing telemetry to create organization endpoint :sparkles: ([b1b2f11](https://github.com/LerianStudio/midaz/commit/b1b2f115607b34777a1024226544f5c0e017b083))


### Bug Fixes

* update infra docker compose to use envs on otel containers and in his yaml config file :bug: ([a6ba7cb](https://github.com/LerianStudio/midaz/commit/a6ba7cbfc07baa095d785c6283c0048243333078))

## [1.27.0-beta.8](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.7...v1.27.0-beta.8) (2024-11-08)


### Features

* added new sub command describe on command asset ([5d14dab](https://github.com/LerianStudio/midaz/commit/5d14dabe4a67a3e97f2cd52fa33f50b27bec782a))
* added new sub command update on command asset ([2edf239](https://github.com/LerianStudio/midaz/commit/2edf2397b13dbbc114937ad3e20192b34931c5a7))
* create rest get by id asset ([059d6a1](https://github.com/LerianStudio/midaz/commit/059d6a187a9c4ef2905249e5dc60527451c3fbec))

## [1.27.0-beta.7](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.6...v1.27.0-beta.7) (2024-11-07)


### Features

* add definitions and config :sparkles: ([a49b010](https://github.com/LerianStudio/midaz/commit/a49b010269122600bdf6ed0fa02a5b6aa9f703d4))
* add repository on command and query :sparkles: ([94d254a](https://github.com/LerianStudio/midaz/commit/94d254ae9e74ce7ac9509e625228eba019b4e7a1))
* adjust wire inject :sparkles: ([ca0ddb4](https://github.com/LerianStudio/midaz/commit/ca0ddb40cd490353126108fe36334241f7cb714c))
* **transaction:** create connection files and add amqp on go.mod :sparkles: ([63f816f](https://github.com/LerianStudio/midaz/commit/63f816fcc7d64b570b8495a1ee338e6891ee520a))
* create mocks based on repositories :sparkles: ([f737239](https://github.com/LerianStudio/midaz/commit/f737239876cf9ad944289e4d3ea1491bf37003dd))
* create producer and consumer repositories :sparkles: ([474d2d0](https://github.com/LerianStudio/midaz/commit/474d2d052a32930b75e4abb3cd1be6dc04da1092))
* create rabbitmq handler on ports/http :sparkles: ([96b6b23](https://github.com/LerianStudio/midaz/commit/96b6b23c9a0e6e8b31ea3329f3b7082a0ecdcb93))
* create test to producer and consumer; :sparkles: ([929d825](https://github.com/LerianStudio/midaz/commit/929d825ba8749aab4520e4dac7a8109125f27952))
* implement handler; :sparkles: ([dc9df25](https://github.com/LerianStudio/midaz/commit/dc9df25a6c770a7f361094bd835ae042ad5a1aec))
* implement on cqrs; :sparkles: ([d122ba6](https://github.com/LerianStudio/midaz/commit/d122ba63d7652188622a7a6795616b19fdc86153))
* implement on routes; :sparkles: ([db9b12f](https://github.com/LerianStudio/midaz/commit/db9b12fc69f37e63e7b2006638acd439d3f51035))
* implement producer and consumer on adapters :sparkles: ([4ff04d4](https://github.com/LerianStudio/midaz/commit/4ff04d4295d7adf12f96815070c2f987bb6cc231))
* implement rabbitmq config, inject and wire; :sparkles: ([5baae29](https://github.com/LerianStudio/midaz/commit/5baae29c08e680c65419c7457ec1adc1ce6d4f9a))
* implement rabbitmq on ledger :sparkles: ([17a9c3d](https://github.com/LerianStudio/midaz/commit/17a9c3da33d2c6a8c9720b7d5d7c550a98b35a04))
* implementation mock :sparkles: ([481e856](https://github.com/LerianStudio/midaz/commit/481e856bf004dc4539c0105cba7bd3d05859c1e5))
* implements producer and consumer with interface and implementation :sparkles: ([5dccc86](https://github.com/LerianStudio/midaz/commit/5dccc86eeb1847dc8dc99d835b6a0fed5888b043))
* init of implementing rabbitmq :sparkles: ([ba9dc6f](https://github.com/LerianStudio/midaz/commit/ba9dc6f567d592ba6628ea61273d970e867b53e6))


### Bug Fixes

* complete conn and health on rabbitmq :bug: ([61d1431](https://github.com/LerianStudio/midaz/commit/61d143170704a8cb35b33395e61812fc31f206f5))
* create new users to separate ledger and transaction :bug: ([24f66c8](https://github.com/LerianStudio/midaz/commit/24f66c8bb43938a5e44206853153542b51a9471c))
* make lint :bug: ([5a7847a](https://github.com/LerianStudio/midaz/commit/5a7847aea01f89d606604c4311e4539347ba26f3))
* make lint; :bug: ([3e55c43](https://github.com/LerianStudio/midaz/commit/3e55c436db91bb34c2f61a3671313eaf449988a9))
* mock :bug: ([5b2d152](https://github.com/LerianStudio/midaz/commit/5b2d152ff987638a36bc643796aa9d755b0e53fc))
* remove producer and consumer from commons :bug: ([fec19a9](https://github.com/LerianStudio/midaz/commit/fec19a901d4af1ec05eaf488ab721c501d2b9714))
* **transaction:** remove test handler :bug: ([8081dcf](https://github.com/LerianStudio/midaz/commit/8081dcf69a654132062a8915cff02b0213765d03))
* **ledger:** remove test handler; :bug: ([2dc3803](https://github.com/LerianStudio/midaz/commit/2dc38035c3db16ea905fd5955eb99788af8edb70))
* remove unusued alias common :bug: ([cdd77f1](https://github.com/LerianStudio/midaz/commit/cdd77f1681cc8a5ef90a8aaf62592ab7aec91b76))
* update amqp :bug: ([b2d6d22](https://github.com/LerianStudio/midaz/commit/b2d6d22d48251c6b377b503dbf848b07e7c09fc9))
* update injection; :bug: ([2da2b58](https://github.com/LerianStudio/midaz/commit/2da2b5808844b9f248c9d557647e5851d815393d))

## [1.27.0-beta.6](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.5...v1.27.0-beta.6) (2024-11-07)


### Features

* created subcommand list of the command asset ([c2d19fc](https://github.com/LerianStudio/midaz/commit/c2d19fc6bdb29b64f9f5435dd8dcb5d23115ad04))

## [1.27.0-beta.5](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.4...v1.27.0-beta.5) (2024-11-07)


### Features

* created asset command, creation of the create subcommand of the asset command ([bdace84](https://github.com/LerianStudio/midaz/commit/bdace84be5e1d8909439a5d91d67cf86e16d6e90))

## [1.27.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.3...v1.27.0-beta.4) (2024-11-06)


### Features

* add grafana-lgtm and open telemetry collector to infra docker-compose :sparkles: ([6351d3b](https://github.com/LerianStudio/midaz/commit/6351d3bc5db24ac09afa693909ee2725c2a5b012))


### Bug Fixes

* uncomment grafana-lgtm and otel-collector on infra docker-compose :bug: ([07dabfd](https://github.com/LerianStudio/midaz/commit/07dabfd64fa09773e04253aa90ca54e90b356623))

## [1.27.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.2...v1.27.0-beta.3) (2024-11-06)


### Features

* create sub command delete from command ledger with test unit of the command delete ([63de66e](https://github.com/LerianStudio/midaz/commit/63de66eff8e604e13bae20d3842c4c6302f93503))
* method delete rest api ledger ([e8917de](https://github.com/LerianStudio/midaz/commit/e8917ded93e7fb3d9bbaa38e66c5734e1fe8b41b))

## [1.27.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.27.0-beta.1...v1.27.0-beta.2) (2024-11-06)


### Features

* create rest update ledger ([b2f8129](https://github.com/LerianStudio/midaz/commit/b2f81295f8773a4d5b4c26e7d306122d1c2f1ee8))
* create sub command update from command ledger ([57fc305](https://github.com/LerianStudio/midaz/commit/57fc305d5bfd7cd6eaab25c651b27c3bb604a02b))


### Bug Fixes

* login via flag no save token local ([656b15a](https://github.com/LerianStudio/midaz/commit/656b15a964a22eb400fae1716b7c10c649283265))

## [1.27.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.26.0...v1.27.0-beta.1) (2024-11-04)


### Bug Fixes

* ah metadata structure is totally optional now, it caused errors when trying to request with null fields in the api ([3dac45f](https://github.com/LerianStudio/midaz/commit/3dac45fea9bd1c2fef7990289d4c33eb5884d182))

## [1.26.1](https://github.com/LerianStudio/midaz/compare/v1.26.0...v1.26.1) (2024-11-05)

## [1.26.0](https://github.com/LerianStudio/midaz/compare/v1.25.0...v1.26.0) (2024-11-01)


### Features

* add account creation endpoint with optional portfolioId :sparkles: ([eb51270](https://github.com/LerianStudio/midaz/commit/eb51270a3c36d32975140a0e6df6188080e31fe1))
* add account update endpoint without portfolioId :sparkles: ([3d7ea8f](https://github.com/LerianStudio/midaz/commit/3d7ea8f754faef82d07fdbc519ebe7595fe1ee89))
* add endpoint for account deleting without portfolioID :sparkles: ([075ba90](https://github.com/LerianStudio/midaz/commit/075ba9032fe35f4ac125807a8e6cf719babcf33b))
* add endpoints for account consulting with optional portfolioID :sparkles: ([910228b](https://github.com/LerianStudio/midaz/commit/910228bc2da1231d3a6f516d39cca63a44dfa787))
* add uuid handler for routes with path parammeters :sparkles: ([f95111e](https://github.com/LerianStudio/midaz/commit/f95111ec4c8dabcb81959b3e0306219fb95080b3))
* added sub command delete from organization ([99dbf17](https://github.com/LerianStudio/midaz/commit/99dbf176bd39cb81585d589158061d633aee65c7))
* added sub command update from organization ([7945691](https://github.com/LerianStudio/midaz/commit/7945691afb1c4c434b7c96cc50c0c200f6a4d513))
* create comamnd ledger and sub command create ([d4a8538](https://github.com/LerianStudio/midaz/commit/d4a85386237e2ca040587591cc7c8a489d9c44dd))
* create rest create ledger ([a0435ac](https://github.com/LerianStudio/midaz/commit/a0435acd44ececb0977c90e92a402809b7348bad))
* create rest list ledger ([88102a2](https://github.com/LerianStudio/midaz/commit/88102a215dbbeec4089560da09f4c644c4743784))
* create sub command describe from ledger and test unit ([418e660](https://github.com/LerianStudio/midaz/commit/418e6600e37cc2ab303b6fe278477c66ef6865f0))
* create sub command list from ledger and test unit with output golden ([3d68791](https://github.com/LerianStudio/midaz/commit/3d68791977bb3ebfd8876de7c75d7c744bcb28f1))
* gitaction to update midaz submodule in midaz-full :sparkles: ([5daafa6](https://github.com/LerianStudio/midaz/commit/5daafa6d397cb975db329ff83f80992903407eb1))
* rest get id command describe ([4e80174](https://github.com/LerianStudio/midaz/commit/4e80174534057e0e3fbcfdce231c66103308946f))
* test unit command create ledger ([93754de](https://github.com/LerianStudio/midaz/commit/93754deae69c2167e4ca9d3bc2def0b1fdd9e8ff))


### Bug Fixes

* add nil validation for status description in account toProto func :bug: ([387b856](https://github.com/LerianStudio/midaz/commit/387b8560029bdd49010e28312da7d0038db16dba))
* remove deleted_at is null condition from account consult endpoints and related functions :bug: ([af6e15a](https://github.com/LerianStudio/midaz/commit/af6e15a4798357991e6c1cca5ba9911c0f987bb3))
* remove portfolioID for duplicated alias validation on create account :bug: ([d043045](https://github.com/LerianStudio/midaz/commit/d0430453e5a548d84fab88e6283c298f78e384f6))
* sec and lint; :bug: ([46bf3b2](https://github.com/LerianStudio/midaz/commit/46bf3b29524b286b5361fcb209c3ec5e84714547))
* **operation:** use parsed uuids :bug: ([0c5eff2](https://github.com/LerianStudio/midaz/commit/0c5eff2c3e0edeac2e76414557e115883f0e2350))
* **transaction:** use parsed uuids :bug: ([dbb19ad](https://github.com/LerianStudio/midaz/commit/dbb19adf62fd400c0685292f2e4d79170c59d248))
* validate duplicated alias when updating account :bug: ([60f19c8](https://github.com/LerianStudio/midaz/commit/60f19c89065800cef5172dc43a9772fb425af1af))

## [1.26.0-beta.14](https://github.com/LerianStudio/midaz/compare/v1.26.0-beta.13...v1.26.0-beta.14) (2024-11-01)


### Features

* create sub command describe from ledger and test unit ([418e660](https://github.com/LerianStudio/midaz/commit/418e6600e37cc2ab303b6fe278477c66ef6865f0))
* rest get id command describe ([4e80174](https://github.com/LerianStudio/midaz/commit/4e80174534057e0e3fbcfdce231c66103308946f))

## [1.26.0-beta.13](https://github.com/LerianStudio/midaz/compare/v1.26.0-beta.12...v1.26.0-beta.13) (2024-11-01)


### Features

* gitaction to update midaz submodule in midaz-full :sparkles: ([5daafa6](https://github.com/LerianStudio/midaz/commit/5daafa6d397cb975db329ff83f80992903407eb1))

## [1.26.0-beta.12](https://github.com/LerianStudio/midaz/compare/v1.26.0-beta.11...v1.26.0-beta.12) (2024-11-01)


### Features

* create rest list ledger ([88102a2](https://github.com/LerianStudio/midaz/commit/88102a215dbbeec4089560da09f4c644c4743784))
* create sub command list from ledger and test unit with output golden ([3d68791](https://github.com/LerianStudio/midaz/commit/3d68791977bb3ebfd8876de7c75d7c744bcb28f1))

## [1.26.0-beta.11](https://github.com/LerianStudio/midaz/compare/v1.26.0-beta.10...v1.26.0-beta.11) (2024-11-01)


### Features

* create comamnd ledger and sub command create ([d4a8538](https://github.com/LerianStudio/midaz/commit/d4a85386237e2ca040587591cc7c8a489d9c44dd))
* create rest create ledger ([a0435ac](https://github.com/LerianStudio/midaz/commit/a0435acd44ececb0977c90e92a402809b7348bad))
* test unit command create ledger ([93754de](https://github.com/LerianStudio/midaz/commit/93754deae69c2167e4ca9d3bc2def0b1fdd9e8ff))

## [1.26.0-beta.10](https://github.com/LerianStudio/midaz/compare/v1.26.0-beta.9...v1.26.0-beta.10) (2024-10-31)


### Features

* add account update endpoint without portfolioId :sparkles: ([3d7ea8f](https://github.com/LerianStudio/midaz/commit/3d7ea8f754faef82d07fdbc519ebe7595fe1ee89))

## [1.26.0-beta.9](https://github.com/LerianStudio/midaz/compare/v1.26.0-beta.8...v1.26.0-beta.9) (2024-10-31)


### Features

* add account creation endpoint with optional portfolioId :sparkles: ([eb51270](https://github.com/LerianStudio/midaz/commit/eb51270a3c36d32975140a0e6df6188080e31fe1))

## [1.26.0-beta.8](https://github.com/LerianStudio/midaz/compare/v1.26.0-beta.7...v1.26.0-beta.8) (2024-10-30)

## [1.26.0-beta.7](https://github.com/LerianStudio/midaz/compare/v1.26.0-beta.6...v1.26.0-beta.7) (2024-10-30)


### Features

* add endpoint for account deleting without portfolioID :sparkles: ([075ba90](https://github.com/LerianStudio/midaz/commit/075ba9032fe35f4ac125807a8e6cf719babcf33b))


### Bug Fixes

* remove deleted_at is null condition from account consult endpoints and related functions :bug: ([af6e15a](https://github.com/LerianStudio/midaz/commit/af6e15a4798357991e6c1cca5ba9911c0f987bb3))

## [1.26.0-beta.6](https://github.com/LerianStudio/midaz/compare/v1.26.0-beta.5...v1.26.0-beta.6) (2024-10-30)


### Bug Fixes

* sec and lint; :bug: ([46bf3b2](https://github.com/LerianStudio/midaz/commit/46bf3b29524b286b5361fcb209c3ec5e84714547))

## [1.26.0-beta.5](https://github.com/LerianStudio/midaz/compare/v1.26.0-beta.4...v1.26.0-beta.5) (2024-10-30)


### Bug Fixes

* remove portfolioID for duplicated alias validation on create account :bug: ([d043045](https://github.com/LerianStudio/midaz/commit/d0430453e5a548d84fab88e6283c298f78e384f6))
* validate duplicated alias when updating account :bug: ([60f19c8](https://github.com/LerianStudio/midaz/commit/60f19c89065800cef5172dc43a9772fb425af1af))

## [1.26.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.26.0-beta.3...v1.26.0-beta.4) (2024-10-30)


### Features

* added sub command delete from organization ([99dbf17](https://github.com/LerianStudio/midaz/commit/99dbf176bd39cb81585d589158061d633aee65c7))

## [1.26.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.26.0-beta.2...v1.26.0-beta.3) (2024-10-30)


### Features

* add uuid handler for routes with path parammeters :sparkles: ([f95111e](https://github.com/LerianStudio/midaz/commit/f95111ec4c8dabcb81959b3e0306219fb95080b3))


### Bug Fixes

* add nil validation for status description in account toProto func :bug: ([387b856](https://github.com/LerianStudio/midaz/commit/387b8560029bdd49010e28312da7d0038db16dba))
* **operation:** use parsed uuids :bug: ([0c5eff2](https://github.com/LerianStudio/midaz/commit/0c5eff2c3e0edeac2e76414557e115883f0e2350))
* **transaction:** use parsed uuids :bug: ([dbb19ad](https://github.com/LerianStudio/midaz/commit/dbb19adf62fd400c0685292f2e4d79170c59d248))

## [1.26.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.26.0-beta.1...v1.26.0-beta.2) (2024-10-30)


### Features

* added sub command update from organization ([7945691](https://github.com/LerianStudio/midaz/commit/7945691afb1c4c434b7c96cc50c0c200f6a4d513))

## [1.26.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.25.0...v1.26.0-beta.1) (2024-10-30)


### Features

* add endpoints for account consulting with optional portfolioID :sparkles: ([910228b](https://github.com/LerianStudio/midaz/commit/910228bc2da1231d3a6f516d39cca63a44dfa787))

## [1.25.0](https://github.com/LerianStudio/midaz/compare/v1.24.0...v1.25.0) (2024-10-29)


### Features

* added sub command list from organization ([32ecea1](https://github.com/LerianStudio/midaz/commit/32ecea1811ace742647b8dfa3ee4b20a69c9a7bb))
* added sub command list from organization ([dfcaab0](https://github.com/LerianStudio/midaz/commit/dfcaab041769dd87f313d1effe21dda384f01286))
* adds new error message for metadata nested structures :sparkles: ([4a7c634](https://github.com/LerianStudio/midaz/commit/4a7c634194f1b614e6754cd94e4d1416716e51b5))
* create command create from organization ([c0742da](https://github.com/LerianStudio/midaz/commit/c0742daa8afa6dd4d3e45a38760a64c9b7559a2c))
* **asset:** create external account if it does not exist during asset creation :sparkles: ([c88b220](https://github.com/LerianStudio/midaz/commit/c88b220e240c0924e1077797ab91d6e05c23472c))
* create rest getby id organization ([3959de5](https://github.com/LerianStudio/midaz/commit/3959de5ccb65804255e96b9c455f7dfbc87563dc))
* create sub command describe from command organization ([af35793](https://github.com/LerianStudio/midaz/commit/af35793c0c1b1f27ab46b735daf48a3ce52c598d))
* Get asset rates - part 2 :sparkles: ([52d5be4](https://github.com/LerianStudio/midaz/commit/52d5be459eba786409cad4b9feee900a8c6451c4))
* Get asset rates :sparkles: ([48c5dec](https://github.com/LerianStudio/midaz/commit/48c5deccfeebf564b55b6e492271f5dde4585055))
* implements custom validators for metadata fields :sparkles: ([005446e](https://github.com/LerianStudio/midaz/commit/005446ef8e5cb6ae6b3aa879586328e28046bd34))
* implements new function to parse Metadata from requests :sparkles: ([d933a58](https://github.com/LerianStudio/midaz/commit/d933a58ee7abee5893399ce3bc19bb25ad7207f7))
* post rest organization ([e7de90d](https://github.com/LerianStudio/midaz/commit/e7de90d241a9d0679a9ea57669784e9b5942a91c))
* update go version 1.22 to 1.23; :sparkles: ([1d32f7e](https://github.com/LerianStudio/midaz/commit/1d32f7eebd3018ad83d2d7f86a0a502d859ff08e))


### Bug Fixes

*  adjust normalization of values ​​with decimal places for remaining :bug: ([fc4f220](https://github.com/LerianStudio/midaz/commit/fc4f22035b622aa88f1c7ebb1652a2da96d278ff))
* add omitempty to all status domain structs :bug: ([c946146](https://github.com/LerianStudio/midaz/commit/c94614651d1a14683cc53808227a2d3c3753b8b7))
* **account:** add organizationID and ledgerID to the grpc account funcs :bug: ([39b29e7](https://github.com/LerianStudio/midaz/commit/39b29e7e41288360a5cffe2a3bbb60e63738f98e))
* add validation for allowReceiving and allowSending status fields in create endpoints for all components :bug: ([3dad79d](https://github.com/LerianStudio/midaz/commit/3dad79d7a5ea23ff26c6976ced69c5083a4c31cf))
* add validation for status fields in create endpoints for all components :bug: ([0779976](https://github.com/LerianStudio/midaz/commit/077997648bdf3f156291208fdc85c30614e2ff93))
* adjust balance_scale can not be inserted on balance_on_hold :bug: ([9482b5a](https://github.com/LerianStudio/midaz/commit/9482b5a11ab75f823cbdbbcf3b279c08716564a4))
* adjust portfolio, allowreceiving and allowsending; :bug: ([4f16cd1](https://github.com/LerianStudio/midaz/commit/4f16cd10c89504d82eeaa3b89ab498178b2be00a))
* adjust transaction model and parse :bug: ([060ff1d](https://github.com/LerianStudio/midaz/commit/060ff1d29a02dc71a1d2a761b10d664c7304fcbd))
* **account:** change error for create account with parent account id not found :bug: ([a2471a9](https://github.com/LerianStudio/midaz/commit/a2471a9bf76401015503e61fab6843f9f092bbca))
* change midaz url :bug: ([acbaf9e](https://github.com/LerianStudio/midaz/commit/acbaf9eb081fab39c7ff1c5e53bd12d2063af5eb))
* fixes file name typo :bug: ([3cbab1a](https://github.com/LerianStudio/midaz/commit/3cbab1ab19171144d33ee16bbaa87f0b925062e1))
* fixes metadata error messages max length parameters :bug: ([d9f334e](https://github.com/LerianStudio/midaz/commit/d9f334ee8c57b8e3d1ac70ccb4479749380ea3c2))
* implements RFC merge patch rules for metadata update :bug: ([7cf7bcd](https://github.com/LerianStudio/midaz/commit/7cf7bcdddad9a5b3fd6e548eae6acae3efa1860c))
* lint :bug: ([9e5ebf1](https://github.com/LerianStudio/midaz/commit/9e5ebf1f3442efbaffcc4df2dbfe3924c37810f0))
* logging entity name for metadata creation error :bug: ([1f70e1b](https://github.com/LerianStudio/midaz/commit/1f70e1b1df0c0a67d092ab882e5f57a15d6f49d0))
* make lint issues; :bug: ([96fc0bf](https://github.com/LerianStudio/midaz/commit/96fc0bfcea8fe44f734b97dec6fddd2a6804d792))
* omits validation error fields if empty :bug: ([313a3cd](https://github.com/LerianStudio/midaz/commit/313a3cd6b60f7dc05f760efbf95b68ffa8885fad))
* omitting empty metadata from responses :bug: ([7878b44](https://github.com/LerianStudio/midaz/commit/7878b44171d326e6cd157c8a4500c17636dca294))
* remove deprecated metadata validation calls :bug: ([549aa99](https://github.com/LerianStudio/midaz/commit/549aa99b25b0d5e98c27c5de7144206bf8b18c6d))
* **auth:** remove ids from the auth init sql insert :bug: ([0965a7b](https://github.com/LerianStudio/midaz/commit/0965a7b2c46498735ba9d2804fb3e704a49154bd))
* resolve G304 CWE-22 potential file inclusion via variable ([91a4350](https://github.com/LerianStudio/midaz/commit/91a43508fda108300aef26d1a5cb9195923ca21b))
* sec and lint; :bug: ([a60cb56](https://github.com/LerianStudio/midaz/commit/a60cb56960d1d3103e9530e890eeaadc3edb587a))
* set new metadata validators for necessary inputs :bug: ([ead05ab](https://github.com/LerianStudio/midaz/commit/ead05ab8c3cd87e0e87e06f5b112a2c235a7f146))
* some adjusts :bug: ([8e90ad8](https://github.com/LerianStudio/midaz/commit/8e90ad877f4d6a4c9384492b465b19abe5c29260))
* update imports; :bug: ([a42bbcf](https://github.com/LerianStudio/midaz/commit/a42bbcf083288eb58c97075ab4bafd7a52286dec))
* update lint; :bug: ([90c929f](https://github.com/LerianStudio/midaz/commit/90c929f6c6cd502acad9aaf70302f8df8970a505))
* update transaction table after on transaction; :bug: ([296fe6e](https://github.com/LerianStudio/midaz/commit/296fe6e0214a05569fb45d014ae81817b2314d9a))
* uses id sent over path to update metadata :bug: ([0918712](https://github.com/LerianStudio/midaz/commit/0918712c9252a3aac93bc6db96cdc2ae879a017f))
* validate share to int :bug: ([db12411](https://github.com/LerianStudio/midaz/commit/db1241131d64ae6bd6ea437b0fec98d3f6ea0332))
* validations to transaction rules :bug: ([769abba](https://github.com/LerianStudio/midaz/commit/769abbae61503e7b916a6246ddc1e6d1155250cc))

## [1.25.0-beta.16](https://github.com/LerianStudio/midaz/compare/v1.25.0-beta.15...v1.25.0-beta.16) (2024-10-29)


### Bug Fixes

* lint :bug: ([9e5ebf1](https://github.com/LerianStudio/midaz/commit/9e5ebf1f3442efbaffcc4df2dbfe3924c37810f0))
* validate share to int :bug: ([db12411](https://github.com/LerianStudio/midaz/commit/db1241131d64ae6bd6ea437b0fec98d3f6ea0332))

## [1.25.0-beta.15](https://github.com/LerianStudio/midaz/compare/v1.25.0-beta.14...v1.25.0-beta.15) (2024-10-29)


### Bug Fixes

* adjust balance_scale can not be inserted on balance_on_hold :bug: ([9482b5a](https://github.com/LerianStudio/midaz/commit/9482b5a11ab75f823cbdbbcf3b279c08716564a4))
* adjust portfolio, allowreceiving and allowsending; :bug: ([4f16cd1](https://github.com/LerianStudio/midaz/commit/4f16cd10c89504d82eeaa3b89ab498178b2be00a))
* update lint; :bug: ([90c929f](https://github.com/LerianStudio/midaz/commit/90c929f6c6cd502acad9aaf70302f8df8970a505))

## [1.25.0-beta.14](https://github.com/LerianStudio/midaz/compare/v1.25.0-beta.13...v1.25.0-beta.14) (2024-10-29)

## [1.25.0-beta.13](https://github.com/LerianStudio/midaz/compare/v1.25.0-beta.12...v1.25.0-beta.13) (2024-10-29)

## [1.25.0-beta.12](https://github.com/LerianStudio/midaz/compare/v1.25.0-beta.11...v1.25.0-beta.12) (2024-10-29)


### Bug Fixes

* add omitempty to all status domain structs :bug: ([c946146](https://github.com/LerianStudio/midaz/commit/c94614651d1a14683cc53808227a2d3c3753b8b7))
* add validation for allowReceiving and allowSending status fields in create endpoints for all components :bug: ([3dad79d](https://github.com/LerianStudio/midaz/commit/3dad79d7a5ea23ff26c6976ced69c5083a4c31cf))
* add validation for status fields in create endpoints for all components :bug: ([0779976](https://github.com/LerianStudio/midaz/commit/077997648bdf3f156291208fdc85c30614e2ff93))

## [1.25.0-beta.11](https://github.com/LerianStudio/midaz/compare/v1.25.0-beta.10...v1.25.0-beta.11) (2024-10-28)


### Features

* update go version 1.22 to 1.23; :sparkles: ([1d32f7e](https://github.com/LerianStudio/midaz/commit/1d32f7eebd3018ad83d2d7f86a0a502d859ff08e))


### Bug Fixes

* adjust transaction model and parse :bug: ([060ff1d](https://github.com/LerianStudio/midaz/commit/060ff1d29a02dc71a1d2a761b10d664c7304fcbd))
* change midaz url :bug: ([acbaf9e](https://github.com/LerianStudio/midaz/commit/acbaf9eb081fab39c7ff1c5e53bd12d2063af5eb))
* sec and lint; :bug: ([a60cb56](https://github.com/LerianStudio/midaz/commit/a60cb56960d1d3103e9530e890eeaadc3edb587a))
* some adjusts :bug: ([8e90ad8](https://github.com/LerianStudio/midaz/commit/8e90ad877f4d6a4c9384492b465b19abe5c29260))
* update imports; :bug: ([a42bbcf](https://github.com/LerianStudio/midaz/commit/a42bbcf083288eb58c97075ab4bafd7a52286dec))
* validations to transaction rules :bug: ([769abba](https://github.com/LerianStudio/midaz/commit/769abbae61503e7b916a6246ddc1e6d1155250cc))

## [1.25.0-beta.10](https://github.com/LerianStudio/midaz/compare/v1.25.0-beta.9...v1.25.0-beta.10) (2024-10-28)


### Features

* **asset:** create external account if it does not exist during asset creation :sparkles: ([c88b220](https://github.com/LerianStudio/midaz/commit/c88b220e240c0924e1077797ab91d6e05c23472c))


### Bug Fixes

* **account:** add organizationID and ledgerID to the grpc account funcs :bug: ([39b29e7](https://github.com/LerianStudio/midaz/commit/39b29e7e41288360a5cffe2a3bbb60e63738f98e))

## [1.25.0-beta.9](https://github.com/LerianStudio/midaz/compare/v1.25.0-beta.8...v1.25.0-beta.9) (2024-10-28)


### Features

* added sub command list from organization ([32ecea1](https://github.com/LerianStudio/midaz/commit/32ecea1811ace742647b8dfa3ee4b20a69c9a7bb))
* create rest getby id organization ([3959de5](https://github.com/LerianStudio/midaz/commit/3959de5ccb65804255e96b9c455f7dfbc87563dc))
* create sub command describe from command organization ([af35793](https://github.com/LerianStudio/midaz/commit/af35793c0c1b1f27ab46b735daf48a3ce52c598d))

## [1.25.0-beta.8](https://github.com/LerianStudio/midaz/compare/v1.25.0-beta.7...v1.25.0-beta.8) (2024-10-28)


### Features

* added sub command list from organization ([dfcaab0](https://github.com/LerianStudio/midaz/commit/dfcaab041769dd87f313d1effe21dda384f01286))

## [1.25.0-beta.7](https://github.com/LerianStudio/midaz/compare/v1.25.0-beta.6...v1.25.0-beta.7) (2024-10-28)


### Features

* adds new error message for metadata nested structures :sparkles: ([4a7c634](https://github.com/LerianStudio/midaz/commit/4a7c634194f1b614e6754cd94e4d1416716e51b5))
* implements custom validators for metadata fields :sparkles: ([005446e](https://github.com/LerianStudio/midaz/commit/005446ef8e5cb6ae6b3aa879586328e28046bd34))


### Bug Fixes

* fixes metadata error messages max length parameters :bug: ([d9f334e](https://github.com/LerianStudio/midaz/commit/d9f334ee8c57b8e3d1ac70ccb4479749380ea3c2))
* omits validation error fields if empty :bug: ([313a3cd](https://github.com/LerianStudio/midaz/commit/313a3cd6b60f7dc05f760efbf95b68ffa8885fad))
* remove deprecated metadata validation calls :bug: ([549aa99](https://github.com/LerianStudio/midaz/commit/549aa99b25b0d5e98c27c5de7144206bf8b18c6d))
* set new metadata validators for necessary inputs :bug: ([ead05ab](https://github.com/LerianStudio/midaz/commit/ead05ab8c3cd87e0e87e06f5b112a2c235a7f146))

## [1.25.0-beta.6](https://github.com/LerianStudio/midaz/compare/v1.25.0-beta.5...v1.25.0-beta.6) (2024-10-25)


### Features

* implements new function to parse Metadata from requests :sparkles: ([d933a58](https://github.com/LerianStudio/midaz/commit/d933a58ee7abee5893399ce3bc19bb25ad7207f7))


### Bug Fixes

* fixes file name typo :bug: ([3cbab1a](https://github.com/LerianStudio/midaz/commit/3cbab1ab19171144d33ee16bbaa87f0b925062e1))
* implements RFC merge patch rules for metadata update :bug: ([7cf7bcd](https://github.com/LerianStudio/midaz/commit/7cf7bcdddad9a5b3fd6e548eae6acae3efa1860c))
* logging entity name for metadata creation error :bug: ([1f70e1b](https://github.com/LerianStudio/midaz/commit/1f70e1b1df0c0a67d092ab882e5f57a15d6f49d0))
* omitting empty metadata from responses :bug: ([7878b44](https://github.com/LerianStudio/midaz/commit/7878b44171d326e6cd157c8a4500c17636dca294))
* uses id sent over path to update metadata :bug: ([0918712](https://github.com/LerianStudio/midaz/commit/0918712c9252a3aac93bc6db96cdc2ae879a017f))

## [1.25.0-beta.5](https://github.com/LerianStudio/midaz/compare/v1.25.0-beta.4...v1.25.0-beta.5) (2024-10-24)


### Bug Fixes

* **auth:** remove ids from the auth init sql insert :bug: ([0965a7b](https://github.com/LerianStudio/midaz/commit/0965a7b2c46498735ba9d2804fb3e704a49154bd))

## [1.25.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.25.0-beta.3...v1.25.0-beta.4) (2024-10-24)


### Bug Fixes

* **account:** change error for create account with parent account id not found :bug: ([a2471a9](https://github.com/LerianStudio/midaz/commit/a2471a9bf76401015503e61fab6843f9f092bbca))

## [1.25.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.25.0-beta.2...v1.25.0-beta.3) (2024-10-24)


### Features

* Get asset rates - part 2 :sparkles: ([52d5be4](https://github.com/LerianStudio/midaz/commit/52d5be459eba786409cad4b9feee900a8c6451c4))
* Get asset rates :sparkles: ([48c5dec](https://github.com/LerianStudio/midaz/commit/48c5deccfeebf564b55b6e492271f5dde4585055))

## [1.25.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.25.0-beta.1...v1.25.0-beta.2) (2024-10-24)


### Features

* create command create from organization ([c0742da](https://github.com/LerianStudio/midaz/commit/c0742daa8afa6dd4d3e45a38760a64c9b7559a2c))
* post rest organization ([e7de90d](https://github.com/LerianStudio/midaz/commit/e7de90d241a9d0679a9ea57669784e9b5942a91c))


### Bug Fixes

* resolve G304 CWE-22 potential file inclusion via variable ([91a4350](https://github.com/LerianStudio/midaz/commit/91a43508fda108300aef26d1a5cb9195923ca21b))

## [1.25.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.24.0...v1.25.0-beta.1) (2024-10-24)


### Bug Fixes

*  adjust normalization of values ​​with decimal places for remaining :bug: ([fc4f220](https://github.com/LerianStudio/midaz/commit/fc4f22035b622aa88f1c7ebb1652a2da96d278ff))
* make lint issues; :bug: ([96fc0bf](https://github.com/LerianStudio/midaz/commit/96fc0bfcea8fe44f734b97dec6fddd2a6804d792))
* update transaction table after on transaction; :bug: ([296fe6e](https://github.com/LerianStudio/midaz/commit/296fe6e0214a05569fb45d014ae81817b2314d9a))

## [1.24.0](https://github.com/LerianStudio/midaz/compare/v1.23.0...v1.24.0) (2024-10-24)


### Features

* add update account with status approved after transfers :sparkles: ([f84ee03](https://github.com/LerianStudio/midaz/commit/f84ee038bd5792fb72b3bcfa12dec6fdcac2ce73))
* Create asset rates - part 2 :sparkles: ([c2d636c](https://github.com/LerianStudio/midaz/commit/c2d636c32a2fd83aeff6005aaeb599994a260b3c))
* Create asset rates :sparkles: ([5e4519f](https://github.com/LerianStudio/midaz/commit/5e4519f6e64e65b094bfff685e4ac0221e3183c7))
* create operation const in commons :sparkles: ([b204230](https://github.com/LerianStudio/midaz/commit/b204230da55f4ebe699f45400bc3dc7350d37e91))
* create update transaction by status; :sparkles: ([181ba8a](https://github.com/LerianStudio/midaz/commit/181ba8a0a621698069f348aabacfd8c741b1ec93))


### Bug Fixes

* add field sizing to onboarding and portfolio domain structs :bug: ([df44228](https://github.com/LerianStudio/midaz/commit/df44228ecdc667a818daf218aefcc0f5012e9821))
* adjust import alias :bug: ([c31d28d](https://github.com/LerianStudio/midaz/commit/c31d28d20c30d72a5c7fe87c77aa2752124c269a))
* adjust import; :bug: ([64b3456](https://github.com/LerianStudio/midaz/commit/64b345697ba0636247e742a27da6251dcb9efc2f))
* adjust some validation to add and remove values from accounts using scale; :bug: ([d59e19d](https://github.com/LerianStudio/midaz/commit/d59e19d33b51cac8592faf991656cfb2fbf78f33))
* **errors:** correcting invalid account type error message :bug: ([4df336d](https://github.com/LerianStudio/midaz/commit/4df336d420438fd4d7d0ec108006aae14afdf5bc))
* update casdoor logger named ping to health check :bug: ([528285e](https://github.com/LerianStudio/midaz/commit/528285e8c2b7421523a94b5b420903813ff3647d))
* update field sizing on onboarding and portfolio domain structs accordingly with rfc :bug: ([d8db53d](https://github.com/LerianStudio/midaz/commit/d8db53d506db76772c273f32df2f2c0875146868))
* update find scale right; :bug: ([6e2b45c](https://github.com/LerianStudio/midaz/commit/6e2b45ca6db1b877c430af60bbb61fe85231a3d9))
* use operations const instead of account type to save operations :bug: ([e74ce4b](https://github.com/LerianStudio/midaz/commit/e74ce4b4703de7a538b4bf277ea3be4e438adb2f))

## [1.24.0-beta.5](https://github.com/LerianStudio/midaz/compare/v1.24.0-beta.4...v1.24.0-beta.5) (2024-10-24)

## [1.24.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.24.0-beta.3...v1.24.0-beta.4) (2024-10-23)


### Features

* add update account with status approved after transfers :sparkles: ([f84ee03](https://github.com/LerianStudio/midaz/commit/f84ee038bd5792fb72b3bcfa12dec6fdcac2ce73))
* create update transaction by status; :sparkles: ([181ba8a](https://github.com/LerianStudio/midaz/commit/181ba8a0a621698069f348aabacfd8c741b1ec93))


### Bug Fixes

* adjust import; :bug: ([64b3456](https://github.com/LerianStudio/midaz/commit/64b345697ba0636247e742a27da6251dcb9efc2f))
* adjust some validation to add and remove values from accounts using scale; :bug: ([d59e19d](https://github.com/LerianStudio/midaz/commit/d59e19d33b51cac8592faf991656cfb2fbf78f33))
* update find scale right; :bug: ([6e2b45c](https://github.com/LerianStudio/midaz/commit/6e2b45ca6db1b877c430af60bbb61fe85231a3d9))

## [1.24.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.24.0-beta.2...v1.24.0-beta.3) (2024-10-23)


### Features

* Create asset rates - part 2 :sparkles: ([c2d636c](https://github.com/LerianStudio/midaz/commit/c2d636c32a2fd83aeff6005aaeb599994a260b3c))
* Create asset rates :sparkles: ([5e4519f](https://github.com/LerianStudio/midaz/commit/5e4519f6e64e65b094bfff685e4ac0221e3183c7))


### Bug Fixes

* adjust import alias :bug: ([c31d28d](https://github.com/LerianStudio/midaz/commit/c31d28d20c30d72a5c7fe87c77aa2752124c269a))

## [1.24.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.24.0-beta.1...v1.24.0-beta.2) (2024-10-23)


### Features

* create operation const in commons :sparkles: ([b204230](https://github.com/LerianStudio/midaz/commit/b204230da55f4ebe699f45400bc3dc7350d37e91))


### Bug Fixes

* update casdoor logger named ping to health check :bug: ([528285e](https://github.com/LerianStudio/midaz/commit/528285e8c2b7421523a94b5b420903813ff3647d))
* use operations const instead of account type to save operations :bug: ([e74ce4b](https://github.com/LerianStudio/midaz/commit/e74ce4b4703de7a538b4bf277ea3be4e438adb2f))

## [1.24.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.23.0...v1.24.0-beta.1) (2024-10-22)


### Bug Fixes

* add field sizing to onboarding and portfolio domain structs :bug: ([df44228](https://github.com/LerianStudio/midaz/commit/df44228ecdc667a818daf218aefcc0f5012e9821))
* **errors:** correcting invalid account type error message :bug: ([4df336d](https://github.com/LerianStudio/midaz/commit/4df336d420438fd4d7d0ec108006aae14afdf5bc))
* update field sizing on onboarding and portfolio domain structs accordingly with rfc :bug: ([d8db53d](https://github.com/LerianStudio/midaz/commit/d8db53d506db76772c273f32df2f2c0875146868))

## [1.23.0](https://github.com/LerianStudio/midaz/compare/v1.22.0...v1.23.0) (2024-10-22)


### Features

* add infra to template; :sparkles: ([f18dca9](https://github.com/LerianStudio/midaz/commit/f18dca99754eb5c0aa71936c075979c27048bb53))
* **logging:** wrap zap logger implementation with otelzap :sparkles: ([d792e65](https://github.com/LerianStudio/midaz/commit/d792e651312d63a3812d1d737710db2f0329e1d3))


### Bug Fixes

* **logging:** add logger sync to server for graceful shutdown :bug: ([c51a4af](https://github.com/LerianStudio/midaz/commit/c51a4af37d9f0f646c6386505138ab9273aeaefd))
* add make set_env on make file; :bug: ([6c6bead](https://github.com/LerianStudio/midaz/commit/6c6bead89a289a52993d7e75b3687753637f4624))
* adjust transaction log; :bug: ([823ec66](https://github.com/LerianStudio/midaz/commit/823ec6643f8e54af7fd47acc434c010b0416fd31))
* change map instantiation; :bug: ([1d3f1e8](https://github.com/LerianStudio/midaz/commit/1d3f1e8178950da03b9320371b140ad752f10cd5))
* **logging:** resolve logging issues for all routes :bug: ([694dadb](https://github.com/LerianStudio/midaz/commit/694dadb29889d2884920260d6e6ac765e5c672dd))
* return bash on make.sh; :bug: ([0b964ba](https://github.com/LerianStudio/midaz/commit/0b964ba235ef81a1ccce11f6d9a8b1f372a50de9))
* **logging:** set capital color level encoder for non-production environments :bug: ([f5f6e73](https://github.com/LerianStudio/midaz/commit/f5f6e7329037de63ba2c6325446c0961eabee4b4))
* some adjusts; :bug: ([9f45958](https://github.com/LerianStudio/midaz/commit/9f45958bba6cec482d582f4ba12acbdfbff6129d))
* **logging:** update sync func being called in zap logger for graceful shutdown :bug: ([4ad1ff2](https://github.com/LerianStudio/midaz/commit/4ad1ff2a653500ae9e9d97280bed0eba2f226f4c))
* uses uuid type instead of string for portfolio creation :bug: ([f1edeef](https://github.com/LerianStudio/midaz/commit/f1edeefe2132c3439c46b41a1f56edcc84b4ccfa))

## [1.23.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.23.0-beta.2...v1.23.0-beta.3) (2024-10-22)


### Features

* add infra to template; :sparkles: ([f18dca9](https://github.com/LerianStudio/midaz/commit/f18dca99754eb5c0aa71936c075979c27048bb53))


### Bug Fixes

* add make set_env on make file; :bug: ([6c6bead](https://github.com/LerianStudio/midaz/commit/6c6bead89a289a52993d7e75b3687753637f4624))
* adjust transaction log; :bug: ([823ec66](https://github.com/LerianStudio/midaz/commit/823ec6643f8e54af7fd47acc434c010b0416fd31))
* change map instantiation; :bug: ([1d3f1e8](https://github.com/LerianStudio/midaz/commit/1d3f1e8178950da03b9320371b140ad752f10cd5))
* return bash on make.sh; :bug: ([0b964ba](https://github.com/LerianStudio/midaz/commit/0b964ba235ef81a1ccce11f6d9a8b1f372a50de9))
* some adjusts; :bug: ([9f45958](https://github.com/LerianStudio/midaz/commit/9f45958bba6cec482d582f4ba12acbdfbff6129d))

## [1.23.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.23.0-beta.1...v1.23.0-beta.2) (2024-10-22)


### Features

* **logging:** wrap zap logger implementation with otelzap :sparkles: ([d792e65](https://github.com/LerianStudio/midaz/commit/d792e651312d63a3812d1d737710db2f0329e1d3))


### Bug Fixes

* **logging:** add logger sync to server for graceful shutdown :bug: ([c51a4af](https://github.com/LerianStudio/midaz/commit/c51a4af37d9f0f646c6386505138ab9273aeaefd))
* **logging:** resolve logging issues for all routes :bug: ([694dadb](https://github.com/LerianStudio/midaz/commit/694dadb29889d2884920260d6e6ac765e5c672dd))
* **logging:** set capital color level encoder for non-production environments :bug: ([f5f6e73](https://github.com/LerianStudio/midaz/commit/f5f6e7329037de63ba2c6325446c0961eabee4b4))
* **logging:** update sync func being called in zap logger for graceful shutdown :bug: ([4ad1ff2](https://github.com/LerianStudio/midaz/commit/4ad1ff2a653500ae9e9d97280bed0eba2f226f4c))

## [1.23.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.22.0...v1.23.0-beta.1) (2024-10-22)


### Bug Fixes

* uses uuid type instead of string for portfolio creation :bug: ([f1edeef](https://github.com/LerianStudio/midaz/commit/f1edeefe2132c3439c46b41a1f56edcc84b4ccfa))

## [1.22.0](https://github.com/LerianStudio/midaz/compare/v1.21.0...v1.22.0) (2024-10-22)


### Features

* implements method to check if a ledger exists by name in an organization :sparkles: ([9737579](https://github.com/LerianStudio/midaz/commit/973757967817027815cc1a5497247af3e26ea587))
* product name required :sparkles: ([e3c4a51](https://github.com/LerianStudio/midaz/commit/e3c4a511ef527de01dd9e4032ca9861fa7273bfc))
* validate account type :sparkles: ([6dd3fa0](https://github.com/LerianStudio/midaz/commit/6dd3fa09e4cd43668ad33eec0f0533e775117e1e))


### Bug Fixes

* error in the logic not respecting the username and password flags ([b76e361](https://github.com/LerianStudio/midaz/commit/b76e3615a06e48140e57ed74c2d6d06db513e60b))
* patch account doesnt return the right data :bug: ([a9c97c2](https://github.com/LerianStudio/midaz/commit/a9c97c2b48b16ae237195777fa8c77d23370e184))
* rename to put on pattern :bug: ([ec8141a](https://github.com/LerianStudio/midaz/commit/ec8141ae8195d6e6f864ee766bca94bf6e90de03))
* sets name as a required field for creating ledgers :bug: ([534cda5](https://github.com/LerianStudio/midaz/commit/534cda5d9203a6c478baf8980dea2e3fc2170eaf))
* sets type as a required field for creating accounts :bug: ([a35044f](https://github.com/LerianStudio/midaz/commit/a35044f7d79b4eb3ecd1476d9ac5527e36617fb1))
* setting cursor input and interactive terminal output ([9b45c14](https://github.com/LerianStudio/midaz/commit/9b45c147a68c5fb030e264b65a3c05f32c8eaa04))
* update some ports on .env :bug: ([b7c58ea](https://github.com/LerianStudio/midaz/commit/b7c58ea75e5c82b8728a785a4b233fa5351c478c))
* uses parsed UUID for organizationID on create ledger :bug: ([b506dc3](https://github.com/LerianStudio/midaz/commit/b506dc3dfe1e1ea8abdf251ca040ab3a6db163ef))
* validates if a ledger with the same name already exists for the same organization :bug: ([08df20b](https://github.com/LerianStudio/midaz/commit/08df20bf4cdd99fc33ce3d273162addb0023afc6))

## [1.22.0-beta.13](https://github.com/LerianStudio/midaz/compare/v1.22.0-beta.12...v1.22.0-beta.13) (2024-10-22)

## [1.22.0-beta.12](https://github.com/LerianStudio/midaz/compare/v1.22.0-beta.11...v1.22.0-beta.12) (2024-10-22)


### Features

* product name required :sparkles: ([e3c4a51](https://github.com/LerianStudio/midaz/commit/e3c4a511ef527de01dd9e4032ca9861fa7273bfc))

## [1.22.0-beta.11](https://github.com/LerianStudio/midaz/compare/v1.22.0-beta.10...v1.22.0-beta.11) (2024-10-22)

## [1.22.0-beta.10](https://github.com/LerianStudio/midaz/compare/v1.22.0-beta.9...v1.22.0-beta.10) (2024-10-22)

## [1.22.0-beta.9](https://github.com/LerianStudio/midaz/compare/v1.22.0-beta.8...v1.22.0-beta.9) (2024-10-22)

## [1.22.0-beta.8](https://github.com/LerianStudio/midaz/compare/v1.22.0-beta.7...v1.22.0-beta.8) (2024-10-22)


### Features

* validate account type :sparkles: ([6dd3fa0](https://github.com/LerianStudio/midaz/commit/6dd3fa09e4cd43668ad33eec0f0533e775117e1e))

## [1.22.0-beta.7](https://github.com/LerianStudio/midaz/compare/v1.22.0-beta.6...v1.22.0-beta.7) (2024-10-21)


### Bug Fixes

* error in the logic not respecting the username and password flags ([b76e361](https://github.com/LerianStudio/midaz/commit/b76e3615a06e48140e57ed74c2d6d06db513e60b))
* setting cursor input and interactive terminal output ([9b45c14](https://github.com/LerianStudio/midaz/commit/9b45c147a68c5fb030e264b65a3c05f32c8eaa04))

## [1.22.0-beta.6](https://github.com/LerianStudio/midaz/compare/v1.22.0-beta.5...v1.22.0-beta.6) (2024-10-21)


### Features

* implements method to check if a ledger exists by name in an organization :sparkles: ([9737579](https://github.com/LerianStudio/midaz/commit/973757967817027815cc1a5497247af3e26ea587))


### Bug Fixes

* uses parsed UUID for organizationID on create ledger :bug: ([b506dc3](https://github.com/LerianStudio/midaz/commit/b506dc3dfe1e1ea8abdf251ca040ab3a6db163ef))
* validates if a ledger with the same name already exists for the same organization :bug: ([08df20b](https://github.com/LerianStudio/midaz/commit/08df20bf4cdd99fc33ce3d273162addb0023afc6))

## [1.22.0-beta.5](https://github.com/LerianStudio/midaz/compare/v1.22.0-beta.4...v1.22.0-beta.5) (2024-10-21)


### Bug Fixes

* rename to put on pattern :bug: ([ec8141a](https://github.com/LerianStudio/midaz/commit/ec8141ae8195d6e6f864ee766bca94bf6e90de03))
* update some ports on .env :bug: ([b7c58ea](https://github.com/LerianStudio/midaz/commit/b7c58ea75e5c82b8728a785a4b233fa5351c478c))

## [1.22.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.22.0-beta.3...v1.22.0-beta.4) (2024-10-21)


### Bug Fixes

* patch account doesnt return the right data :bug: ([a9c97c2](https://github.com/LerianStudio/midaz/commit/a9c97c2b48b16ae237195777fa8c77d23370e184))

## [1.22.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.22.0-beta.2...v1.22.0-beta.3) (2024-10-21)


### Bug Fixes

* sets type as a required field for creating accounts :bug: ([a35044f](https://github.com/LerianStudio/midaz/commit/a35044f7d79b4eb3ecd1476d9ac5527e36617fb1))

## [1.22.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.22.0-beta.1...v1.22.0-beta.2) (2024-10-21)

## [1.22.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.21.0...v1.22.0-beta.1) (2024-10-21)


### Bug Fixes

* sets name as a required field for creating ledgers :bug: ([534cda5](https://github.com/LerianStudio/midaz/commit/534cda5d9203a6c478baf8980dea2e3fc2170eaf))

## [1.21.0](https://github.com/LerianStudio/midaz/compare/v1.20.0...v1.21.0) (2024-10-18)


### Features

* create command login mode term, browser :sparkles: ([80a9326](https://github.com/LerianStudio/midaz/commit/80a932663d5e2747b59fb740a46d828a852e10a9))
* create transaction using json based on dsl struct :sparkles: ([a2552ed](https://github.com/LerianStudio/midaz/commit/a2552ed74b40e92963b265f1defd69ab32d43482))


### Bug Fixes

* change midaz code owner file :bug: ([8f5e2c2](https://github.com/LerianStudio/midaz/commit/8f5e2c202fe6fa9ae9140ac032ad304f97ab34a6))
* make sec and lint; :bug: ([93a3dd6](https://github.com/LerianStudio/midaz/commit/93a3dd6eddc6c3f032a7e5844355b81c93ccbf5f))
* sets entityID as a required field for portfolio creation :bug: ([5a74f7d](https://github.com/LerianStudio/midaz/commit/5a74f7d381061ec677ed6c3d1887793deb4fd7ca))
* sets name as a required field for portfolio creation :bug: ([ef35811](https://github.com/LerianStudio/midaz/commit/ef358115fba637d8430897b58f87eb5cc2295fb2))
* some update to add and sub accounts; adjust validate accounts balance; :bug: ([e705cbd](https://github.com/LerianStudio/midaz/commit/e705cbd3db086d400ae6440b8b21870d3c28cd49))
* update postman; :bug: ([4931c51](https://github.com/LerianStudio/midaz/commit/4931c5117ca48a4bbd4471f61e0f1d987da9c60b))
* updates to get all accounts :bug: ([f536a9a](https://github.com/LerianStudio/midaz/commit/f536a9a5ee1b949b60dded1b3ae7709b4e219d55))

## [1.21.0-beta.5](https://github.com/LerianStudio/midaz/compare/v1.21.0-beta.4...v1.21.0-beta.5) (2024-10-18)


### Bug Fixes

* change midaz code owner file :bug: ([8f5e2c2](https://github.com/LerianStudio/midaz/commit/8f5e2c202fe6fa9ae9140ac032ad304f97ab34a6))

## [1.21.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.21.0-beta.3...v1.21.0-beta.4) (2024-10-18)


### Features

* create transaction using json based on dsl struct :sparkles: ([a2552ed](https://github.com/LerianStudio/midaz/commit/a2552ed74b40e92963b265f1defd69ab32d43482))


### Bug Fixes

* make sec and lint; :bug: ([93a3dd6](https://github.com/LerianStudio/midaz/commit/93a3dd6eddc6c3f032a7e5844355b81c93ccbf5f))
* some update to add and sub accounts; adjust validate accounts balance; :bug: ([e705cbd](https://github.com/LerianStudio/midaz/commit/e705cbd3db086d400ae6440b8b21870d3c28cd49))
* update postman; :bug: ([4931c51](https://github.com/LerianStudio/midaz/commit/4931c5117ca48a4bbd4471f61e0f1d987da9c60b))
* updates to get all accounts :bug: ([f536a9a](https://github.com/LerianStudio/midaz/commit/f536a9a5ee1b949b60dded1b3ae7709b4e219d55))

## [1.21.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.21.0-beta.2...v1.21.0-beta.3) (2024-10-18)


### Bug Fixes

* sets entityID as a required field for portfolio creation :bug: ([5a74f7d](https://github.com/LerianStudio/midaz/commit/5a74f7d381061ec677ed6c3d1887793deb4fd7ca))

## [1.21.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.21.0-beta.1...v1.21.0-beta.2) (2024-10-18)


### Features

* create command login mode term, browser :sparkles: ([80a9326](https://github.com/LerianStudio/midaz/commit/80a932663d5e2747b59fb740a46d828a852e10a9))

## [1.21.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.20.0...v1.21.0-beta.1) (2024-10-18)


### Bug Fixes

* sets name as a required field for portfolio creation :bug: ([ef35811](https://github.com/LerianStudio/midaz/commit/ef358115fba637d8430897b58f87eb5cc2295fb2))

## [1.20.0](https://github.com/LerianStudio/midaz/compare/v1.19.0...v1.20.0) (2024-10-18)


### Features

* validate code for all types :sparkles: ([c0e7b31](https://github.com/LerianStudio/midaz/commit/c0e7b3179839c720f24ce2da00e5c20172616f10))


### Bug Fixes

* update error message for invalid path parameters :bug: ([5942994](https://github.com/LerianStudio/midaz/commit/5942994ed9f31d3a3257f46a829463df2e607d93))

## [1.20.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.20.0-beta.1...v1.20.0-beta.2) (2024-10-18)


### Features

* validate code for all types :sparkles: ([c0e7b31](https://github.com/LerianStudio/midaz/commit/c0e7b3179839c720f24ce2da00e5c20172616f10))

## [1.20.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.19.0...v1.20.0-beta.1) (2024-10-18)


### Bug Fixes

* update error message for invalid path parameters :bug: ([5942994](https://github.com/LerianStudio/midaz/commit/5942994ed9f31d3a3257f46a829463df2e607d93))

## [1.19.0](https://github.com/LerianStudio/midaz/compare/v1.18.0...v1.19.0) (2024-10-18)


### Features

* adds UUID handler for routes with path parameters :sparkles: ([6153896](https://github.com/LerianStudio/midaz/commit/6153896bc83e0d3048a7223f89eafe6b6f2deae3))
* adds validation error for invalid path parameters :sparkles: ([270ecfd](https://github.com/LerianStudio/midaz/commit/270ecfdc7aa14040aefa29ab09710aa6274acce9))
* implement get operation by portfolio :sparkles: ([1e9322f](https://github.com/LerianStudio/midaz/commit/1e9322f8257672d95d850739609af87c673d7b56))
* implements handler for parsing UUID path parameters :sparkles: ([6baa571](https://github.com/LerianStudio/midaz/commit/6baa571275c876ab48760f882e48a400bd892196))
* initialize CLI with root and version commands :sparkles: ([6ebff8a](https://github.com/LerianStudio/midaz/commit/6ebff8a40ba097b0eaa4feb1106ebc29a5ba84dc))
* require code :sparkles: ([40d1bbd](https://github.com/LerianStudio/midaz/commit/40d1bbd7f54c85aaab279e36754274df93d12a34))


### Bug Fixes

* add log; :bug: ([3a71282](https://github.com/LerianStudio/midaz/commit/3a712820a16ede4cd50cdc1729c5abf0507950b0))
* add parentheses on find name or asset query; :bug: ([9b71d2e](https://github.com/LerianStudio/midaz/commit/9b71d2ee9bafba37b0eb9e1a0f328b5d10036d1e))
* add required in asset_code; :bug: ([d2481eb](https://github.com/LerianStudio/midaz/commit/d2481ebf4d3007df5337394c151360aca28ee69a))
* adjust to validate if exists code on assets; :bug: ([583890a](https://github.com/LerianStudio/midaz/commit/583890a6c1d178b95b41666a91600a60d3053123))
* asset validate create before to ledger_id :bug: ([da0a22a](https://github.com/LerianStudio/midaz/commit/da0a22a38f57c6d8217e8511abb07592523c822f))
* better formatting for error message :bug: ([d7135ff](https://github.com/LerianStudio/midaz/commit/d7135ff90f50f154a95928829142a37226be7629))
* create validation on code to certify that asset_code exist on assets before insert in accounts; :bug: ([2375963](https://github.com/LerianStudio/midaz/commit/2375963e26657972f22ac714c905775bdf0ed5d5))
* go sec and lint; :bug: ([4d22c8c](https://github.com/LerianStudio/midaz/commit/4d22c8c5be0f6498c5305ed01e1121efbe4e8987))
* Invalid code format validation :bug: ([e8383ca](https://github.com/LerianStudio/midaz/commit/e8383cac7957d1f0d63ce20f71534052ab1e8703))
* Invalid code format validation :bug: ([4dfe76c](https://github.com/LerianStudio/midaz/commit/4dfe76c1092412a129a60b09d408f71d8a59dca0))
* remove asset_code validation on account :bug: ([05b89c5](https://github.com/LerianStudio/midaz/commit/05b89c52266d1e067ffc429d29405d49f50762dc))
* remove copyloopvar and perfsprint; :bug: ([a181709](https://github.com/LerianStudio/midaz/commit/a1817091640de24bad22e43eaddccd86b21dcf82))
* remove goconst :bug: ([707be65](https://github.com/LerianStudio/midaz/commit/707be656984aaea2c839be70f6c7c17e84375866))
* remove unique constraint on database in code and reference on accounts; :bug: ([926ca9b](https://github.com/LerianStudio/midaz/commit/926ca9b758d7e69611afa903c035fa01218b108f))
* resolve conflicts :bug: ([bc4b697](https://github.com/LerianStudio/midaz/commit/bc4b697c2e50cd1ec3cd41e0f96cb933a17b6a79))
* uses parsed uuid while creating asset :bug: ([333bf49](https://github.com/LerianStudio/midaz/commit/333bf4921d3f2fd48156ead07ac8b1b29d88d5fa))
* uses parsed uuid while deleting ledger by id :bug: ([8dc3a97](https://github.com/LerianStudio/midaz/commit/8dc3a97f8c859a6948cad099cd61888c8c016bee))
* uses parsed uuid while deleting organization :bug: ([866170a](https://github.com/LerianStudio/midaz/commit/866170a1d2bb849fc1ed002a9aed99d7ee43eecb))
* uses parsed uuid while getting all organization ledgers :bug: ([2260a33](https://github.com/LerianStudio/midaz/commit/2260a331e381d452bcab942f9f06864c60444f52))
* uses parsed uuid while getting and updating a ledger :bug: ([ad1bcae](https://github.com/LerianStudio/midaz/commit/ad1bcae482d2939c8e828b169b566d3a13be95cd))
* uses parsed uuid while retrieving all assets from a ledger :bug: ([aadf885](https://github.com/LerianStudio/midaz/commit/aadf8852154726bd4aef2e3295221b5472236ed9))
* uses parsed uuid while retrieving and updating asset :bug: ([9c8b3a2](https://github.com/LerianStudio/midaz/commit/9c8b3a2f9747117e5149f3a515c8a5b582db4942))
* uses parsed uuid while retrieving organization :bug: ([e2d2848](https://github.com/LerianStudio/midaz/commit/e2d284808c9c1d95d3d1192be2e4ba3e613318dc))
* uses UUID to find asset :bug: ([381ba21](https://github.com/LerianStudio/midaz/commit/381ba2178633863f17cffb327a7ab2276926ce0d))

## [1.19.0-beta.7](https://github.com/LerianStudio/midaz/compare/v1.19.0-beta.6...v1.19.0-beta.7) (2024-10-18)


### Features

* adds UUID handler for routes with path parameters :sparkles: ([6153896](https://github.com/LerianStudio/midaz/commit/6153896bc83e0d3048a7223f89eafe6b6f2deae3))
* adds validation error for invalid path parameters :sparkles: ([270ecfd](https://github.com/LerianStudio/midaz/commit/270ecfdc7aa14040aefa29ab09710aa6274acce9))
* implements handler for parsing UUID path parameters :sparkles: ([6baa571](https://github.com/LerianStudio/midaz/commit/6baa571275c876ab48760f882e48a400bd892196))


### Bug Fixes

* better formatting for error message :bug: ([d7135ff](https://github.com/LerianStudio/midaz/commit/d7135ff90f50f154a95928829142a37226be7629))
* remove asset_code validation on account :bug: ([05b89c5](https://github.com/LerianStudio/midaz/commit/05b89c52266d1e067ffc429d29405d49f50762dc))
* uses parsed uuid while creating asset :bug: ([333bf49](https://github.com/LerianStudio/midaz/commit/333bf4921d3f2fd48156ead07ac8b1b29d88d5fa))
* uses parsed uuid while deleting ledger by id :bug: ([8dc3a97](https://github.com/LerianStudio/midaz/commit/8dc3a97f8c859a6948cad099cd61888c8c016bee))
* uses parsed uuid while deleting organization :bug: ([866170a](https://github.com/LerianStudio/midaz/commit/866170a1d2bb849fc1ed002a9aed99d7ee43eecb))
* uses parsed uuid while getting all organization ledgers :bug: ([2260a33](https://github.com/LerianStudio/midaz/commit/2260a331e381d452bcab942f9f06864c60444f52))
* uses parsed uuid while getting and updating a ledger :bug: ([ad1bcae](https://github.com/LerianStudio/midaz/commit/ad1bcae482d2939c8e828b169b566d3a13be95cd))
* uses parsed uuid while retrieving all assets from a ledger :bug: ([aadf885](https://github.com/LerianStudio/midaz/commit/aadf8852154726bd4aef2e3295221b5472236ed9))
* uses parsed uuid while retrieving and updating asset :bug: ([9c8b3a2](https://github.com/LerianStudio/midaz/commit/9c8b3a2f9747117e5149f3a515c8a5b582db4942))
* uses parsed uuid while retrieving organization :bug: ([e2d2848](https://github.com/LerianStudio/midaz/commit/e2d284808c9c1d95d3d1192be2e4ba3e613318dc))
* uses UUID to find asset :bug: ([381ba21](https://github.com/LerianStudio/midaz/commit/381ba2178633863f17cffb327a7ab2276926ce0d))

## [1.19.0-beta.6](https://github.com/LerianStudio/midaz/compare/v1.19.0-beta.5...v1.19.0-beta.6) (2024-10-18)


### Bug Fixes

* asset validate create before to ledger_id :bug: ([da0a22a](https://github.com/LerianStudio/midaz/commit/da0a22a38f57c6d8217e8511abb07592523c822f))

## [1.19.0-beta.5](https://github.com/LerianStudio/midaz/compare/v1.19.0-beta.4...v1.19.0-beta.5) (2024-10-18)


### Features

* require code :sparkles: ([40d1bbd](https://github.com/LerianStudio/midaz/commit/40d1bbd7f54c85aaab279e36754274df93d12a34))

## [1.19.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.19.0-beta.3...v1.19.0-beta.4) (2024-10-18)


### Bug Fixes

* add log; :bug: ([3a71282](https://github.com/LerianStudio/midaz/commit/3a712820a16ede4cd50cdc1729c5abf0507950b0))
* add parentheses on find name or asset query; :bug: ([9b71d2e](https://github.com/LerianStudio/midaz/commit/9b71d2ee9bafba37b0eb9e1a0f328b5d10036d1e))
* add required in asset_code; :bug: ([d2481eb](https://github.com/LerianStudio/midaz/commit/d2481ebf4d3007df5337394c151360aca28ee69a))
* adjust to validate if exists code on assets; :bug: ([583890a](https://github.com/LerianStudio/midaz/commit/583890a6c1d178b95b41666a91600a60d3053123))
* create validation on code to certify that asset_code exist on assets before insert in accounts; :bug: ([2375963](https://github.com/LerianStudio/midaz/commit/2375963e26657972f22ac714c905775bdf0ed5d5))
* go sec and lint; :bug: ([4d22c8c](https://github.com/LerianStudio/midaz/commit/4d22c8c5be0f6498c5305ed01e1121efbe4e8987))
* remove copyloopvar and perfsprint; :bug: ([a181709](https://github.com/LerianStudio/midaz/commit/a1817091640de24bad22e43eaddccd86b21dcf82))
* remove goconst :bug: ([707be65](https://github.com/LerianStudio/midaz/commit/707be656984aaea2c839be70f6c7c17e84375866))
* remove unique constraint on database in code and reference on accounts; :bug: ([926ca9b](https://github.com/LerianStudio/midaz/commit/926ca9b758d7e69611afa903c035fa01218b108f))

## [1.19.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.19.0-beta.2...v1.19.0-beta.3) (2024-10-18)


### Bug Fixes

* Invalid code format validation :bug: ([e8383ca](https://github.com/LerianStudio/midaz/commit/e8383cac7957d1f0d63ce20f71534052ab1e8703))
* Invalid code format validation :bug: ([4dfe76c](https://github.com/LerianStudio/midaz/commit/4dfe76c1092412a129a60b09d408f71d8a59dca0))

## [1.19.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.19.0-beta.1...v1.19.0-beta.2) (2024-10-17)

## [1.19.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.18.0...v1.19.0-beta.1) (2024-10-17)


### Features

* implement get operation by portfolio :sparkles: ([1e9322f](https://github.com/LerianStudio/midaz/commit/1e9322f8257672d95d850739609af87c673d7b56))
* initialize CLI with root and version commands :sparkles: ([6ebff8a](https://github.com/LerianStudio/midaz/commit/6ebff8a40ba097b0eaa4feb1106ebc29a5ba84dc))


### Bug Fixes

* resolve conflicts :bug: ([bc4b697](https://github.com/LerianStudio/midaz/commit/bc4b697c2e50cd1ec3cd41e0f96cb933a17b6a79))

## [1.18.0](https://github.com/LerianStudio/midaz/compare/v1.17.0...v1.18.0) (2024-10-16)


### Features

* implement patch operation :sparkles: ([d4c6e5c](https://github.com/LerianStudio/midaz/commit/d4c6e5c3823b44b6b3466342f9cc6c24f21e3e05))


### Bug Fixes

* filters if any required fields are missing and returns a customized error message :bug: ([7f6c95a](https://github.com/LerianStudio/midaz/commit/7f6c95a4e388f9edb110f19d8ad5f4ca01b1a7ab))
* sets legalName and legalDocument as required fields for creating or updating an organization :bug: ([1dd238d](https://github.com/LerianStudio/midaz/commit/1dd238d77b68e0e847adc0861deb526588a9049e))
* update .env.example to transaction access accounts on grpc on ledger :bug: ([a643cc6](https://github.com/LerianStudio/midaz/commit/a643cc61c79678f1a1ae91d5eb623f6de04ee2d6))

## [1.18.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.18.0-beta.1...v1.18.0-beta.2) (2024-10-16)


### Features

* implement patch operation :sparkles: ([d4c6e5c](https://github.com/LerianStudio/midaz/commit/d4c6e5c3823b44b6b3466342f9cc6c24f21e3e05))

## [1.18.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.17.0...v1.18.0-beta.1) (2024-10-16)


### Bug Fixes

* filters if any required fields are missing and returns a customized error message :bug: ([7f6c95a](https://github.com/LerianStudio/midaz/commit/7f6c95a4e388f9edb110f19d8ad5f4ca01b1a7ab))
* sets legalName and legalDocument as required fields for creating or updating an organization :bug: ([1dd238d](https://github.com/LerianStudio/midaz/commit/1dd238d77b68e0e847adc0861deb526588a9049e))
* update .env.example to transaction access accounts on grpc on ledger :bug: ([a643cc6](https://github.com/LerianStudio/midaz/commit/a643cc61c79678f1a1ae91d5eb623f6de04ee2d6))

## [1.17.0](https://github.com/LerianStudio/midaz/compare/v1.16.0...v1.17.0) (2024-10-16)


### Bug Fixes

* update scripts to set variable on collection instead of environment :bug: ([e2a52dc](https://github.com/LerianStudio/midaz/commit/e2a52dc5da8b89d5999bae90da292ebce10729cd))

## [1.17.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.16.0...v1.17.0-beta.1) (2024-10-16)


### Bug Fixes

* update scripts to set variable on collection instead of environment :bug: ([e2a52dc](https://github.com/LerianStudio/midaz/commit/e2a52dc5da8b89d5999bae90da292ebce10729cd))

## [1.16.0](https://github.com/LerianStudio/midaz/compare/v1.15.0...v1.16.0) (2024-10-16)


### Features

* implement get operation by portfolio :sparkles: ([35702ae](https://github.com/LerianStudio/midaz/commit/35702ae99ed667a001a317f8932796d6e540d32a))


### Bug Fixes

* add error treatment when extracting dsl file from header and get creating buffer error :bug: ([807a706](https://github.com/LerianStudio/midaz/commit/807a706a810f0b729e43472abaa93db5d96675be))
* add solution to avoid nolint:gocyclo in business error messages handler :bug: ([a293625](https://github.com/LerianStudio/midaz/commit/a293625ea937c6a7ccecf94a254933673bf50816))
* adjust centralized errors name to comply with stylecheck and other lint issues :bug: ([a06361d](https://github.com/LerianStudio/midaz/commit/a06361da717d4330445ab589b5fd9bf800d18743))
* adjust reference to errors in common instead of http package :bug: ([c0deae2](https://github.com/LerianStudio/midaz/commit/c0deae240ba53530ec9750bacf6cf23862c127dc))

## [1.16.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.16.0-beta.3...v1.16.0-beta.4) (2024-10-16)

## [1.16.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.16.0-beta.2...v1.16.0-beta.3) (2024-10-15)


### Features

* implement get operation by portfolio :sparkles: ([35702ae](https://github.com/LerianStudio/midaz/commit/35702ae99ed667a001a317f8932796d6e540d32a))

## [1.16.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.16.0-beta.1...v1.16.0-beta.2) (2024-10-15)

## [1.16.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.15.0...v1.16.0-beta.1) (2024-10-15)


### Bug Fixes

* add error treatment when extracting dsl file from header and get creating buffer error :bug: ([807a706](https://github.com/LerianStudio/midaz/commit/807a706a810f0b729e43472abaa93db5d96675be))
* add solution to avoid nolint:gocyclo in business error messages handler :bug: ([a293625](https://github.com/LerianStudio/midaz/commit/a293625ea937c6a7ccecf94a254933673bf50816))
* adjust centralized errors name to comply with stylecheck and other lint issues :bug: ([a06361d](https://github.com/LerianStudio/midaz/commit/a06361da717d4330445ab589b5fd9bf800d18743))
* adjust reference to errors in common instead of http package :bug: ([c0deae2](https://github.com/LerianStudio/midaz/commit/c0deae240ba53530ec9750bacf6cf23862c127dc))

## [1.15.0](https://github.com/LerianStudio/midaz/compare/v1.14.1...v1.15.0) (2024-10-14)


### Features

* add new funcs to solve some problems separately :sparkles: ([c88dd61](https://github.com/LerianStudio/midaz/commit/c88dd6163837534d330211f9233262a986f6ac15))
* create a func process account on handler to update accounts :sparkles: ([67ba62b](https://github.com/LerianStudio/midaz/commit/67ba62bf124584cf47caae1bae9c4729294d0ac3))
* create func on validate to adjust values to send to update :sparkles: ([8ffe1ce](https://github.com/LerianStudio/midaz/commit/8ffe1ce51a9c408fbcbe2625900b3a3a85cd91fe))
* create some validations func to scale, undoscale and so on... :sparkles: ([3471f2b](https://github.com/LerianStudio/midaz/commit/3471f2b9cb34c20573695a23e746dfc27bfd6fe5))
* dsl validations nuances to sources and distribute :sparkles: ([07452a7](https://github.com/LerianStudio/midaz/commit/07452a79f724399c8f3f42a8181ec7de4532032c))
* implement auth on transaction :sparkles: ([a183909](https://github.com/LerianStudio/midaz/commit/a183909b1122ff19dbddb08b3fa51771a4c68738))
* implement get operation by account :sparkles: ([9137bc1](https://github.com/LerianStudio/midaz/commit/9137bc126f902d23a482e1894995c2cf9bb77230))
* implement get operations by account :sparkles: ([1a75922](https://github.com/LerianStudio/midaz/commit/1a7592273ef8e11382c37ff1f78ee921007ef319))
* implement get operations by portfolio :sparkles: ([966e5c5](https://github.com/LerianStudio/midaz/commit/966e5c5f198381081a9f3a403c7e74c007f80785))
* implement new validations to accounts and dsl and save on operations :sparkles: ([53b7a3a](https://github.com/LerianStudio/midaz/commit/53b7a3a673ff7d3fcdb2eee3498239a1d20e3c29))
* implement token to call grpc :sparkles: ([b1fc617](https://github.com/LerianStudio/midaz/commit/b1fc617c95a1aecfefd35a48ef6f069b08397e77))
* implement update account method; change name account to client when get new account proto client; :sparkles: ([5aae505](https://github.com/LerianStudio/midaz/commit/5aae5050878c28bce20937240dea0ed5efe1cbf0))


### Bug Fixes

* accept only [@external](https://github.com/external) accounts to be negative values :bug: ([909eb23](https://github.com/LerianStudio/midaz/commit/909eb23613df8b284190fa490802239dcd256ebc))
* add auth on the new route :bug: ([ed51df9](https://github.com/LerianStudio/midaz/commit/ed51df902e852469c29d6b4a9e37f772515ed180))
* add field boolean to help to know if is from or to struct :bug: ([898fa5d](https://github.com/LerianStudio/midaz/commit/898fa5dbe14352a3381dfc9c58e9d95b2e15b1c4))
* go lint :bug: ([b692801](https://github.com/LerianStudio/midaz/commit/b692801bbb49b4492db0863a05774fa66a2a2746))
* golang sec G601 (CWE-118): Implicit memory aliasing in for loop. (Confidence: MEDIUM, Severity: MEDIUM) :bug: ([9517777](https://github.com/LerianStudio/midaz/commit/9517777b5a37364718351be3824477175ffadafd))
* merge develop :bug: ([cdaf00d](https://github.com/LerianStudio/midaz/commit/cdaf00d73368bf410cd757e66970d90115a6b258))
* remove fmt.sprintf :bug: ([1ba33f8](https://github.com/LerianStudio/midaz/commit/1ba33f82cfc9a44e7b92b13b75c064107d656f02))
* rename OperationHandler alias :bug: ([866c122](https://github.com/LerianStudio/midaz/commit/866c1222ce1ce14e6524495e9a98f986c334027a))
* update some validate erros. :bug: ([623b4d9](https://github.com/LerianStudio/midaz/commit/623b4d9ebdeb436cdab03faa740e906132b7122a))

## [1.15.0-beta.5](https://github.com/LerianStudio/midaz/compare/v1.15.0-beta.4...v1.15.0-beta.5) (2024-10-14)


### Features

* implement auth on transaction :sparkles: ([a183909](https://github.com/LerianStudio/midaz/commit/a183909b1122ff19dbddb08b3fa51771a4c68738))
* implement token to call grpc :sparkles: ([b1fc617](https://github.com/LerianStudio/midaz/commit/b1fc617c95a1aecfefd35a48ef6f069b08397e77))


### Bug Fixes

* accept only [@external](https://github.com/external) accounts to be negative values :bug: ([909eb23](https://github.com/LerianStudio/midaz/commit/909eb23613df8b284190fa490802239dcd256ebc))
* add auth on the new route :bug: ([ed51df9](https://github.com/LerianStudio/midaz/commit/ed51df902e852469c29d6b4a9e37f772515ed180))
* remove fmt.sprintf :bug: ([1ba33f8](https://github.com/LerianStudio/midaz/commit/1ba33f82cfc9a44e7b92b13b75c064107d656f02))

## [1.15.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.15.0-beta.3...v1.15.0-beta.4) (2024-10-14)


### Features

* implement get operation by account :sparkles: ([9137bc1](https://github.com/LerianStudio/midaz/commit/9137bc126f902d23a482e1894995c2cf9bb77230))

## [1.15.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.15.0-beta.2...v1.15.0-beta.3) (2024-10-11)


### Features

* add new funcs to solve some problems separately :sparkles: ([c88dd61](https://github.com/LerianStudio/midaz/commit/c88dd6163837534d330211f9233262a986f6ac15))
* create a func process account on handler to update accounts :sparkles: ([67ba62b](https://github.com/LerianStudio/midaz/commit/67ba62bf124584cf47caae1bae9c4729294d0ac3))
* create func on validate to adjust values to send to update :sparkles: ([8ffe1ce](https://github.com/LerianStudio/midaz/commit/8ffe1ce51a9c408fbcbe2625900b3a3a85cd91fe))
* create some validations func to scale, undoscale and so on... :sparkles: ([3471f2b](https://github.com/LerianStudio/midaz/commit/3471f2b9cb34c20573695a23e746dfc27bfd6fe5))
* dsl validations nuances to sources and distribute :sparkles: ([07452a7](https://github.com/LerianStudio/midaz/commit/07452a79f724399c8f3f42a8181ec7de4532032c))
* implement new validations to accounts and dsl and save on operations :sparkles: ([53b7a3a](https://github.com/LerianStudio/midaz/commit/53b7a3a673ff7d3fcdb2eee3498239a1d20e3c29))
* implement update account method; change name account to client when get new account proto client; :sparkles: ([5aae505](https://github.com/LerianStudio/midaz/commit/5aae5050878c28bce20937240dea0ed5efe1cbf0))


### Bug Fixes

* add field boolean to help to know if is from or to struct :bug: ([898fa5d](https://github.com/LerianStudio/midaz/commit/898fa5dbe14352a3381dfc9c58e9d95b2e15b1c4))
* go lint :bug: ([b692801](https://github.com/LerianStudio/midaz/commit/b692801bbb49b4492db0863a05774fa66a2a2746))
* golang sec G601 (CWE-118): Implicit memory aliasing in for loop. (Confidence: MEDIUM, Severity: MEDIUM) :bug: ([9517777](https://github.com/LerianStudio/midaz/commit/9517777b5a37364718351be3824477175ffadafd))
* merge develop :bug: ([cdaf00d](https://github.com/LerianStudio/midaz/commit/cdaf00d73368bf410cd757e66970d90115a6b258))
* update some validate erros. :bug: ([623b4d9](https://github.com/LerianStudio/midaz/commit/623b4d9ebdeb436cdab03faa740e906132b7122a))

## [1.15.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.15.0-beta.1...v1.15.0-beta.2) (2024-10-11)


### Features

* implement get operations by portfolio :sparkles: ([966e5c5](https://github.com/LerianStudio/midaz/commit/966e5c5f198381081a9f3a403c7e74c007f80785))

## [1.15.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.14.1...v1.15.0-beta.1) (2024-10-10)


### Features

* implement get operations by account :sparkles: ([1a75922](https://github.com/LerianStudio/midaz/commit/1a7592273ef8e11382c37ff1f78ee921007ef319))


### Bug Fixes

* rename OperationHandler alias :bug: ([866c122](https://github.com/LerianStudio/midaz/commit/866c1222ce1ce14e6524495e9a98f986c334027a))

## [1.14.1](https://github.com/LerianStudio/midaz/compare/v1.14.0...v1.14.1) (2024-10-10)

## [1.14.1-beta.4](https://github.com/LerianStudio/midaz/compare/v1.14.1-beta.3...v1.14.1-beta.4) (2024-10-08)

## [1.14.1-beta.3](https://github.com/LerianStudio/midaz/compare/v1.14.1-beta.2...v1.14.1-beta.3) (2024-10-08)

## [1.14.1-beta.2](https://github.com/LerianStudio/midaz/compare/v1.14.1-beta.1...v1.14.1-beta.2) (2024-10-08)

## [1.14.1-beta.1](https://github.com/LerianStudio/midaz/compare/v1.14.0...v1.14.1-beta.1) (2024-10-08)

## [1.14.0](https://github.com/LerianStudio/midaz/compare/v1.13.0...v1.14.0) (2024-10-07)


### Features

* add authorization to Postman requests and implement new transaction route wip :sparkles: ([91afb3f](https://github.com/LerianStudio/midaz/commit/91afb3f26c6d4912a669120ab074f14011b88d10))
* add default enforcer adapter and token fields on casdoor init json also add init sql file to casdoor db :sparkles: ([6bb997b](https://github.com/LerianStudio/midaz/commit/6bb997b7ef3b564be1869d1ffef1e142f6236c7d))
* add permission check to ledger :sparkles: ([352a6c2](https://github.com/LerianStudio/midaz/commit/352a6c295aa57e0ebc4c9df52a36ce8beb6db811))
* add permission check to the ledger grpc routes :sparkles: ([1e4a81f](https://github.com/LerianStudio/midaz/commit/1e4a81f14a3187c0b9de88017a2bb25262494bf5))
* add permission check to the ledger routes :sparkles: ([4ce5162](https://github.com/LerianStudio/midaz/commit/4ce5162df5c06018bb9552168fb02c250768cad5))
* adjusts to create operations based on transaction in dsl :sparkles: ([7ca7f04](https://github.com/LerianStudio/midaz/commit/7ca7f04f3e651d584223b0956b60751e89ecc671))
* implement get transaction by id :sparkles: ([a9f1935](https://github.com/LerianStudio/midaz/commit/a9f193516313d16e8ed349b7f469001a479fa40a))
* Implement UpdateTransaction and GetAllTTransactions :sparkles: ([d2c0e5d](https://github.com/LerianStudio/midaz/commit/d2c0e5d0a729f67973e8328220fe12e6ab2ffdc3))
* insert operations on database after insert transaction :sparkles: ([cc03f5e](https://github.com/LerianStudio/midaz/commit/cc03f5ed7c2e09437d6faa7e0bac9aae73ceda9e))


### Bug Fixes

* add chartofaccounts in dsl struct :bug: ([92325c2](https://github.com/LerianStudio/midaz/commit/92325c23dfcc5c707f7048d94dd7f6147373169a))
* fix lint name and import sorting issues :bug: ([aeb2a87](https://github.com/LerianStudio/midaz/commit/aeb2a8788ef0af33958ffd8de0c58b7f54d9d6a6))
* insert import reflect :bug: ([f1574e6](https://github.com/LerianStudio/midaz/commit/f1574e660a1ac0d4f833daaddc345d1e72609257))
* load transaction after patch :bug: ([456f880](https://github.com/LerianStudio/midaz/commit/456f88076c703a55d28ac3178382134afefadbe2))
* remove db scan position :bug: ([0129bd0](https://github.com/LerianStudio/midaz/commit/0129bd09ec839881813cf8bbc1aed492d73d20da))
* rename get-transaction to get-id-transaction filename :bug: ([96cda1f](https://github.com/LerianStudio/midaz/commit/96cda1f8e7910a27aa9195bcc77317660347367a))
* update proto address and port from ledger and transaction env example :bug: ([95a4f6a](https://github.com/LerianStudio/midaz/commit/95a4f6ac11d37029d4926dcad4026bc6139b5268))
* update slice operation to operations :bug: ([0954fe9](https://github.com/LerianStudio/midaz/commit/0954fe9f9766c8437e222526baa45add2163da2d))
* update subcomands version :bug: ([483348c](https://github.com/LerianStudio/midaz/commit/483348c83b6b56858887cb1c8d49142d25b1cdec))
* validate omitempty from productId for create and update account :bug: ([a6fd703](https://github.com/LerianStudio/midaz/commit/a6fd703f9b5e8ecd4a08fabe2731e387b1206139))

## [1.14.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.14.0-beta.3...v1.14.0-beta.4) (2024-10-07)


### Features

* Implement UpdateTransaction and GetAllTTransactions :sparkles: ([d2c0e5d](https://github.com/LerianStudio/midaz/commit/d2c0e5d0a729f67973e8328220fe12e6ab2ffdc3))


### Bug Fixes

* load transaction after patch :bug: ([456f880](https://github.com/LerianStudio/midaz/commit/456f88076c703a55d28ac3178382134afefadbe2))
* rename get-transaction to get-id-transaction filename :bug: ([96cda1f](https://github.com/LerianStudio/midaz/commit/96cda1f8e7910a27aa9195bcc77317660347367a))

## [1.14.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.14.0-beta.2...v1.14.0-beta.3) (2024-10-07)


### Features

* add authorization to Postman requests and implement new transaction route wip :sparkles: ([91afb3f](https://github.com/LerianStudio/midaz/commit/91afb3f26c6d4912a669120ab074f14011b88d10))
* add default enforcer adapter and token fields on casdoor init json also add init sql file to casdoor db :sparkles: ([6bb997b](https://github.com/LerianStudio/midaz/commit/6bb997b7ef3b564be1869d1ffef1e142f6236c7d))
* add permission check to ledger :sparkles: ([352a6c2](https://github.com/LerianStudio/midaz/commit/352a6c295aa57e0ebc4c9df52a36ce8beb6db811))
* add permission check to the ledger grpc routes :sparkles: ([1e4a81f](https://github.com/LerianStudio/midaz/commit/1e4a81f14a3187c0b9de88017a2bb25262494bf5))
* add permission check to the ledger routes :sparkles: ([4ce5162](https://github.com/LerianStudio/midaz/commit/4ce5162df5c06018bb9552168fb02c250768cad5))


### Bug Fixes

* fix lint name and import sorting issues :bug: ([aeb2a87](https://github.com/LerianStudio/midaz/commit/aeb2a8788ef0af33958ffd8de0c58b7f54d9d6a6))
* update proto address and port from ledger and transaction env example :bug: ([95a4f6a](https://github.com/LerianStudio/midaz/commit/95a4f6ac11d37029d4926dcad4026bc6139b5268))
* validate omitempty from productId for create and update account :bug: ([a6fd703](https://github.com/LerianStudio/midaz/commit/a6fd703f9b5e8ecd4a08fabe2731e387b1206139))

## [1.14.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.14.0-beta.1...v1.14.0-beta.2) (2024-10-04)


### Features

* adjusts to create operations based on transaction in dsl :sparkles: ([7ca7f04](https://github.com/LerianStudio/midaz/commit/7ca7f04f3e651d584223b0956b60751e89ecc671))
* insert operations on database after insert transaction :sparkles: ([cc03f5e](https://github.com/LerianStudio/midaz/commit/cc03f5ed7c2e09437d6faa7e0bac9aae73ceda9e))


### Bug Fixes

* add chartofaccounts in dsl struct :bug: ([92325c2](https://github.com/LerianStudio/midaz/commit/92325c23dfcc5c707f7048d94dd7f6147373169a))
* insert import reflect :bug: ([f1574e6](https://github.com/LerianStudio/midaz/commit/f1574e660a1ac0d4f833daaddc345d1e72609257))
* remove db scan position :bug: ([0129bd0](https://github.com/LerianStudio/midaz/commit/0129bd09ec839881813cf8bbc1aed492d73d20da))
* update slice operation to operations :bug: ([0954fe9](https://github.com/LerianStudio/midaz/commit/0954fe9f9766c8437e222526baa45add2163da2d))
* update subcomands version :bug: ([483348c](https://github.com/LerianStudio/midaz/commit/483348c83b6b56858887cb1c8d49142d25b1cdec))

## [1.14.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.13.0...v1.14.0-beta.1) (2024-10-04)


### Features

* implement get transaction by id :sparkles: ([a9f1935](https://github.com/LerianStudio/midaz/commit/a9f193516313d16e8ed349b7f469001a479fa40a))

## [1.13.0](https://github.com/LerianStudio/midaz/compare/v1.12.0...v1.13.0) (2024-10-02)


### Features

* create grpc account in adapter :sparkles: ([78dbddb](https://github.com/LerianStudio/midaz/commit/78dbddb255c0dd73c74e32c4a049d59af88f6a04))
* create operation postgres crud to use with transaction ([0b541a4](https://github.com/LerianStudio/midaz/commit/0b541a48086bc8336085bee3e71606bd1b55d13f))
* create transaction constant :sparkles: ([4f5a03b](https://github.com/LerianStudio/midaz/commit/4f5a03b920961e33a76d96ead2c05500f97020f8))
* implements transaction api using grcp to get account on ledger :sparkles: ([7b19915](https://github.com/LerianStudio/midaz/commit/7b199150850a41d5a1bb80b725d7bc8db296e10a))


### Bug Fixes

* account proto class updated with all fields. :bug: ([0f00bb7](https://github.com/LerianStudio/midaz/commit/0f00bb79be7fb9ec20723c4f56cd607e6ef144ad))
* add lib :bug: ([55f0aa0](https://github.com/LerianStudio/midaz/commit/55f0aa0fea1b40cce38da9d35e296e66daf15d5c))
* adjust account proto in common to improve requests and responses on ledger :bug: ([844d994](https://github.com/LerianStudio/midaz/commit/844d9949171b04860fc14eef888a0d2732c63bb2))
* adjust to slice to use append instead use index. :bug: ([990c426](https://github.com/LerianStudio/midaz/commit/990c426f87a485790c6c586aadd35b5ac71bf32f))
* create transaction  on postgresql :bug: ([688a16c](https://github.com/LerianStudio/midaz/commit/688a16cc5eb56b99b071b1f21e6e43c6f8758b01))
* insert grpc address and port in environment :bug: ([7813ae3](https://github.com/LerianStudio/midaz/commit/7813ae3dc6df15e7cf5a56c344676e76e930297b))
* insert ledger grpc address and port into transaction .env :bug: ([4be3771](https://github.com/LerianStudio/midaz/commit/4be377158d02369b317f478ccf333ea043bd4573))
* make sec, format, tidy and lint :bug: ([11b9d97](https://github.com/LerianStudio/midaz/commit/11b9d973c405f839a9fc64bcbe1e5a6828345260))
* mongdb connection and wire to save metadata of transaction :bug: ([05f19a5](https://github.com/LerianStudio/midaz/commit/05f19a55ae0b4b241101a865fc464eff203fc5b6))
* remove account http api reference :bug: ([8189389](https://github.com/LerianStudio/midaz/commit/8189389fe7d39dd3dd182c79923a4d1e593dd944))
* remove defer because command always be executed before the connection is even used. :bug: ([a5e4d36](https://github.com/LerianStudio/midaz/commit/a5e4d3612123a24ddcb3eec0741116e48f294a1f))
* remove exemples of dsl gold :bug: ([1daa033](https://github.com/LerianStudio/midaz/commit/1daa03307fbb105d95fdad20cecc37d092bf9838))
* rename .env.exemple to .env.example and update go.sum :bug: ([b6a2a2d](https://github.com/LerianStudio/midaz/commit/b6a2a2dd8fba36b808fd4efc09cdcc3b53d5e708))
* some operation adjust :bug: ([0ab9fa3](https://github.com/LerianStudio/midaz/commit/0ab9fa3b0248e0a0c9a6d1f25b5e5dcfd0bd1d65))
* update convert uint64 make sec alert :bug: ([3779924](https://github.com/LerianStudio/midaz/commit/3779924a809686cb28f9013aa71f6b6611f063e6))
* update docker compose ledger and transaction to add bridge to use grpc call account :bug: ([4115eb1](https://github.com/LerianStudio/midaz/commit/4115eb1e3522751b875c9bab5ad679d8d8912332))
* update grpc accounts proto reference on transaction and some adjusts to improve readable :bug: ([9930082](https://github.com/LerianStudio/midaz/commit/99300826c63355d9bb8b419d0ff1931fcc63e83a))
* update grpc accounts proto reference on transaction and some adjusts to improve readable pt. 2 :bug: ([11e5c71](https://github.com/LerianStudio/midaz/commit/11e5c71576980b9059444a9708abcf430ede85bd))
* update inject and wire :bug: ([8026c16](https://github.com/LerianStudio/midaz/commit/8026c1653921062738a9a6f3f64ca9907c811daf))

## [1.13.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.12.0...v1.13.0-beta.1) (2024-10-02)


### Features

* create grpc account in adapter :sparkles: ([78dbddb](https://github.com/LerianStudio/midaz/commit/78dbddb255c0dd73c74e32c4a049d59af88f6a04))
* create operation postgres crud to use with transaction ([0b541a4](https://github.com/LerianStudio/midaz/commit/0b541a48086bc8336085bee3e71606bd1b55d13f))
* create transaction constant :sparkles: ([4f5a03b](https://github.com/LerianStudio/midaz/commit/4f5a03b920961e33a76d96ead2c05500f97020f8))
* implements transaction api using grcp to get account on ledger :sparkles: ([7b19915](https://github.com/LerianStudio/midaz/commit/7b199150850a41d5a1bb80b725d7bc8db296e10a))


### Bug Fixes

* account proto class updated with all fields. :bug: ([0f00bb7](https://github.com/LerianStudio/midaz/commit/0f00bb79be7fb9ec20723c4f56cd607e6ef144ad))
* add lib :bug: ([55f0aa0](https://github.com/LerianStudio/midaz/commit/55f0aa0fea1b40cce38da9d35e296e66daf15d5c))
* adjust account proto in common to improve requests and responses on ledger :bug: ([844d994](https://github.com/LerianStudio/midaz/commit/844d9949171b04860fc14eef888a0d2732c63bb2))
* adjust to slice to use append instead use index. :bug: ([990c426](https://github.com/LerianStudio/midaz/commit/990c426f87a485790c6c586aadd35b5ac71bf32f))
* create transaction  on postgresql :bug: ([688a16c](https://github.com/LerianStudio/midaz/commit/688a16cc5eb56b99b071b1f21e6e43c6f8758b01))
* insert grpc address and port in environment :bug: ([7813ae3](https://github.com/LerianStudio/midaz/commit/7813ae3dc6df15e7cf5a56c344676e76e930297b))
* insert ledger grpc address and port into transaction .env :bug: ([4be3771](https://github.com/LerianStudio/midaz/commit/4be377158d02369b317f478ccf333ea043bd4573))
* make sec, format, tidy and lint :bug: ([11b9d97](https://github.com/LerianStudio/midaz/commit/11b9d973c405f839a9fc64bcbe1e5a6828345260))
* mongdb connection and wire to save metadata of transaction :bug: ([05f19a5](https://github.com/LerianStudio/midaz/commit/05f19a55ae0b4b241101a865fc464eff203fc5b6))
* remove account http api reference :bug: ([8189389](https://github.com/LerianStudio/midaz/commit/8189389fe7d39dd3dd182c79923a4d1e593dd944))
* remove defer because command always be executed before the connection is even used. :bug: ([a5e4d36](https://github.com/LerianStudio/midaz/commit/a5e4d3612123a24ddcb3eec0741116e48f294a1f))
* remove exemples of dsl gold :bug: ([1daa033](https://github.com/LerianStudio/midaz/commit/1daa03307fbb105d95fdad20cecc37d092bf9838))
* rename .env.exemple to .env.example and update go.sum :bug: ([b6a2a2d](https://github.com/LerianStudio/midaz/commit/b6a2a2dd8fba36b808fd4efc09cdcc3b53d5e708))
* some operation adjust :bug: ([0ab9fa3](https://github.com/LerianStudio/midaz/commit/0ab9fa3b0248e0a0c9a6d1f25b5e5dcfd0bd1d65))
* update convert uint64 make sec alert :bug: ([3779924](https://github.com/LerianStudio/midaz/commit/3779924a809686cb28f9013aa71f6b6611f063e6))
* update docker compose ledger and transaction to add bridge to use grpc call account :bug: ([4115eb1](https://github.com/LerianStudio/midaz/commit/4115eb1e3522751b875c9bab5ad679d8d8912332))
* update grpc accounts proto reference on transaction and some adjusts to improve readable :bug: ([9930082](https://github.com/LerianStudio/midaz/commit/99300826c63355d9bb8b419d0ff1931fcc63e83a))
* update grpc accounts proto reference on transaction and some adjusts to improve readable pt. 2 :bug: ([11e5c71](https://github.com/LerianStudio/midaz/commit/11e5c71576980b9059444a9708abcf430ede85bd))
* update inject and wire :bug: ([8026c16](https://github.com/LerianStudio/midaz/commit/8026c1653921062738a9a6f3f64ca9907c811daf))

## [1.12.0](https://github.com/LerianStudio/midaz/compare/v1.11.0...v1.12.0) (2024-09-27)


### Features

* create auth postman collections and environments ([206ffb1](https://github.com/LerianStudio/midaz/commit/206ffb14845f78a98180d72eafc02c4b281b43a1))
* create casdoor base infrastructure ✨ ([1d10d20](https://github.com/LerianStudio/midaz/commit/1d10d20a52df2d4f7e95b752eecd513c56565dca))


### Bug Fixes

* update postman and environments :bug: ([3f4d97e](https://github.com/LerianStudio/midaz/commit/3f4d97e7d3692ad30d8f0fe2dda55ddb44fd5e8b))

## [1.12.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.11.1-beta.2...v1.12.0-beta.1) (2024-09-27)


### Features

* create auth postman collections and environments ([206ffb1](https://github.com/LerianStudio/midaz/commit/206ffb14845f78a98180d72eafc02c4b281b43a1))
* create casdoor base infrastructure ✨ ([1d10d20](https://github.com/LerianStudio/midaz/commit/1d10d20a52df2d4f7e95b752eecd513c56565dca))


### Bug Fixes

* update postman and environments :bug: ([3f4d97e](https://github.com/LerianStudio/midaz/commit/3f4d97e7d3692ad30d8f0fe2dda55ddb44fd5e8b))

## [1.11.1-beta.2](https://github.com/LerianStudio/midaz/compare/v1.11.1-beta.1...v1.11.1-beta.2) (2024-09-26)

## [1.11.1-beta.1](https://github.com/LerianStudio/midaz/compare/v1.11.0...v1.11.1-beta.1) (2024-09-26)

## [1.11.0](https://github.com/LerianStudio/midaz/compare/v1.10.1...v1.11.0) (2024-09-23)

## [1.11.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.10.1...v1.11.0-beta.1) (2024-09-23)

## [1.10.1](https://github.com/LerianStudio/midaz/compare/v1.10.0...v1.10.1) (2024-09-19)

## [1.10.0](https://github.com/LerianStudio/midaz/compare/v1.9.0...v1.10.0) (2024-09-19)


### Features

* add grpc port to midaz on 50051 to run togheter with fiber :sparkles: ([a9c4551](https://github.com/LerianStudio/midaz/commit/a9c45514be5239593b9a26d1838d140c372d3836))
* add midaz version :sparkles: ([27c56aa](https://github.com/LerianStudio/midaz/commit/27c56aac4aaeffbdd6093a69dbc80e84ea9331ee))
* add proto url, address :sparkles: ([c92ee9b](https://github.com/LerianStudio/midaz/commit/c92ee9bc2649a3c46963027e067c4eed4dddade4))
* add version onn .env file :sparkles: ([fdfdac3](https://github.com/LerianStudio/midaz/commit/fdfdac3bded8767307d7f1e3d68a3c76e5803aa8))
* create new method listbyalias to find accounts based on transaction dsl info :sparkles: ([113c00c](https://github.com/LerianStudio/midaz/commit/113c00c2b64f2577f01460b1e4a017d3750f16ea))
* create new route and server grpc and remove old account class :sparkles: ([c5d9101](https://github.com/LerianStudio/midaz/commit/c5d91011efbc8f0dca1c32091747a36abe3d6039))
* generate new query to search account by ids :sparkles: ([aa5d147](https://github.com/LerianStudio/midaz/commit/aa5d147151fdbc814a41e7ba58496f8c3bce2989))
* grpc server starting with http sever togheter :sparkles: ([6d12e14](https://github.com/LerianStudio/midaz/commit/6d12e140d21b28fe70d2f339a05cba4744cbce60))
* update account by id and get account by alias by grpc :sparkles: ([bf98e11](https://github.com/LerianStudio/midaz/commit/bf98e11eba0e8a33eddd52e1cde4226deb5af872))


### Bug Fixes

* add -d on docker compose up :bug: ([0322e13](https://github.com/LerianStudio/midaz/commit/0322e13cf0cbbc1693cd21352ccb6f142b71d835))
* add clean-up step for existing backup folder in PostgreSQL replica service in docker-compose ([28be466](https://github.com/LerianStudio/midaz/commit/28be466b7dda2f3dd100b73452c90d93ca574eda))
* adjust grpc account service :bug: ([2679e9b](https://github.com/LerianStudio/midaz/commit/2679e9bfe2d94fcc201e5672cec1f86feca5eb95))
* change print error to return error :bug: ([2e28f92](https://github.com/LerianStudio/midaz/commit/2e28f9251b91fcfcd77a33492219f27f0bedb5b0))
* ensure pg_basebackup runs if directory or postgresql.conf file is missing ([9f9742e](https://github.com/LerianStudio/midaz/commit/9f9742e39fe223a7cda85252935ea0d1cbbf6b81))
* go sec and go lint :bug: ([8a91b07](https://github.com/LerianStudio/midaz/commit/8a91b0746257afe7f4c4dc1ad6ce367b6f019cba))
* remove fiber print startup :bug: ([d47dd20](https://github.com/LerianStudio/midaz/commit/d47dd20ba5c888860b9c07fceb4e4ff2b432a167))
* reorganize some class and update wire. :bug: ([af0836b](https://github.com/LerianStudio/midaz/commit/af0836b86395b840b895eea7f1c256b04c5c7d17))
* update version place in log :bug: ([83980a8](https://github.com/LerianStudio/midaz/commit/83980a8aee40884cb317914c40d89e13c12f6a68))

## [1.10.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.10.0-beta.1...v1.10.0-beta.2) (2024-09-19)


### Features

* add grpc port to midaz on 50051 to run togheter with fiber :sparkles: ([a9c4551](https://github.com/LerianStudio/midaz/commit/a9c45514be5239593b9a26d1838d140c372d3836))
* add midaz version :sparkles: ([27c56aa](https://github.com/LerianStudio/midaz/commit/27c56aac4aaeffbdd6093a69dbc80e84ea9331ee))
* add proto url, address :sparkles: ([c92ee9b](https://github.com/LerianStudio/midaz/commit/c92ee9bc2649a3c46963027e067c4eed4dddade4))
* add version onn .env file :sparkles: ([fdfdac3](https://github.com/LerianStudio/midaz/commit/fdfdac3bded8767307d7f1e3d68a3c76e5803aa8))
* create new method listbyalias to find accounts based on transaction dsl info :sparkles: ([113c00c](https://github.com/LerianStudio/midaz/commit/113c00c2b64f2577f01460b1e4a017d3750f16ea))
* create new route and server grpc and remove old account class :sparkles: ([c5d9101](https://github.com/LerianStudio/midaz/commit/c5d91011efbc8f0dca1c32091747a36abe3d6039))
* generate new query to search account by ids :sparkles: ([aa5d147](https://github.com/LerianStudio/midaz/commit/aa5d147151fdbc814a41e7ba58496f8c3bce2989))
* grpc server starting with http sever togheter :sparkles: ([6d12e14](https://github.com/LerianStudio/midaz/commit/6d12e140d21b28fe70d2f339a05cba4744cbce60))
* update account by id and get account by alias by grpc :sparkles: ([bf98e11](https://github.com/LerianStudio/midaz/commit/bf98e11eba0e8a33eddd52e1cde4226deb5af872))


### Bug Fixes

* add -d on docker compose up :bug: ([0322e13](https://github.com/LerianStudio/midaz/commit/0322e13cf0cbbc1693cd21352ccb6f142b71d835))
* adjust grpc account service :bug: ([2679e9b](https://github.com/LerianStudio/midaz/commit/2679e9bfe2d94fcc201e5672cec1f86feca5eb95))
* change print error to return error :bug: ([2e28f92](https://github.com/LerianStudio/midaz/commit/2e28f9251b91fcfcd77a33492219f27f0bedb5b0))
* go sec and go lint :bug: ([8a91b07](https://github.com/LerianStudio/midaz/commit/8a91b0746257afe7f4c4dc1ad6ce367b6f019cba))
* remove fiber print startup :bug: ([d47dd20](https://github.com/LerianStudio/midaz/commit/d47dd20ba5c888860b9c07fceb4e4ff2b432a167))
* reorganize some class and update wire. :bug: ([af0836b](https://github.com/LerianStudio/midaz/commit/af0836b86395b840b895eea7f1c256b04c5c7d17))
* update version place in log :bug: ([83980a8](https://github.com/LerianStudio/midaz/commit/83980a8aee40884cb317914c40d89e13c12f6a68))

## [1.10.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.9.1-beta.1...v1.10.0-beta.1) (2024-09-17)


### Bug Fixes

* add clean-up step for existing backup folder in PostgreSQL replica service in docker-compose ([28be466](https://github.com/LerianStudio/midaz/commit/28be466b7dda2f3dd100b73452c90d93ca574eda))
* ensure pg_basebackup runs if directory or postgresql.conf file is missing ([9f9742e](https://github.com/LerianStudio/midaz/commit/9f9742e39fe223a7cda85252935ea0d1cbbf6b81))

## [1.9.1-beta.1](https://github.com/LerianStudio/midaz/compare/v1.9.0...v1.9.1-beta.1) (2024-09-17)

## [1.9.0](https://github.com/LerianStudio/midaz/compare/v1.8.0...v1.9.0) (2024-09-16)


### Bug Fixes

* adjust cast of int to uint64 because gosec G115 :bug: ([d1d62fb](https://github.com/LerianStudio/midaz/commit/d1d62fb2f0e76a96dce841d6018abd40e3d88655))
* Fixing the ory ports - creating organization and group namespace :bug: ([b4a72b4](https://github.com/LerianStudio/midaz/commit/b4a72b4f5aedc2b8763286ffcdad894af3094e01))
* return statements should not be cuddled if block has more than two lines (wsl) :bug: ([136a780](https://github.com/LerianStudio/midaz/commit/136a780f27bb8f2604461efd058b8208029458ad))
* updated go.mod and go.sum :bug: ([f8ef00c](https://github.com/LerianStudio/midaz/commit/f8ef00c1d41d68223cdc75780f8a1058cfefac48))

## [1.9.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.9.0-beta.3...v1.9.0-beta.4) (2024-09-16)


### Bug Fixes

* adjust cast of int to uint64 because gosec G115 :bug: ([d1d62fb](https://github.com/LerianStudio/midaz/commit/d1d62fb2f0e76a96dce841d6018abd40e3d88655))
* return statements should not be cuddled if block has more than two lines (wsl) :bug: ([136a780](https://github.com/LerianStudio/midaz/commit/136a780f27bb8f2604461efd058b8208029458ad))

## [1.9.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.9.0-beta.2...v1.9.0-beta.3) (2024-09-16)


### Bug Fixes

* updated go.mod and go.sum :bug: ([f8ef00c](https://github.com/LerianStudio/midaz/commit/f8ef00c1d41d68223cdc75780f8a1058cfefac48))

## [1.9.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.9.0-beta.1...v1.9.0-beta.2) (2024-09-16)

## [1.9.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.8.0...v1.9.0-beta.1) (2024-07-02)


### Bug Fixes

* Fixing the ory ports - creating organization and group namespace :bug: ([b4a72b4](https://github.com/LerianStudio/midaz/commit/b4a72b4f5aedc2b8763286ffcdad894af3094e01))

## [1.8.0](https://github.com/LerianStudio/midaz/compare/v1.7.0...v1.8.0) (2024-06-05)


### Features

* add transaction templates ([a55b583](https://github.com/LerianStudio/midaz/commit/a55b5839944e385a94037c20aae5e8b9a415a503))
* init transaction ([b696d05](https://github.com/LerianStudio/midaz/commit/b696d05af93b45841987cb56a6e3bd85fdc7ff90))


### Bug Fixes

* add field UseMetadata  to use on query on mongodb when not use metadata field remove limit and skip to get all :bug: ([fce6bfb](https://github.com/LerianStudio/midaz/commit/fce6bfb2e9132a14205a90dda6164c7eaf7e97f4))
* make lint, sec and tests :bug: ([bb4621b](https://github.com/LerianStudio/midaz/commit/bb4621bc8a5a10a03f9312c9ca52a7cacdac6444))
* update test and change QueryHeader path :bug: ([c8b539f](https://github.com/LerianStudio/midaz/commit/c8b539f4b049633e6e6ad7e76b4d990e22c943f6))

## [1.8.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.7.0...v1.8.0-beta.1) (2024-06-05)


### Features

* add transaction templates ([a55b583](https://github.com/LerianStudio/midaz/commit/a55b5839944e385a94037c20aae5e8b9a415a503))
* init transaction ([b696d05](https://github.com/LerianStudio/midaz/commit/b696d05af93b45841987cb56a6e3bd85fdc7ff90))


### Bug Fixes

* add field UseMetadata  to use on query on mongodb when not use metadata field remove limit and skip to get all :bug: ([fce6bfb](https://github.com/LerianStudio/midaz/commit/fce6bfb2e9132a14205a90dda6164c7eaf7e97f4))
* make lint, sec and tests :bug: ([bb4621b](https://github.com/LerianStudio/midaz/commit/bb4621bc8a5a10a03f9312c9ca52a7cacdac6444))
* update test and change QueryHeader path :bug: ([c8b539f](https://github.com/LerianStudio/midaz/commit/c8b539f4b049633e6e6ad7e76b4d990e22c943f6))

## [1.7.0](https://github.com/LerianStudio/midaz/compare/v1.6.0...v1.7.0) (2024-06-05)


### Features

* Keto Stack Included in Docker Compose file - Auth ([c5c2831](https://github.com/LerianStudio/midaz/commit/c5c28311b661948c922e541cc618e30bcf878313))
* Keto Stack Included in Docker Compose file - Auth ([7be883f](https://github.com/LerianStudio/midaz/commit/7be883fb0a7851d6798eeadfbc79938d20ba4129))


### Bug Fixes

* add comments :bug: ([dfd765f](https://github.com/LerianStudio/midaz/commit/dfd765fab6f1c860879e096eeb2f9527e998d820))

## [1.7.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.6.0...v1.7.0-beta.1) (2024-06-05)


### Features

* Keto Stack Included in Docker Compose file - Auth ([c5c2831](https://github.com/LerianStudio/midaz/commit/c5c28311b661948c922e541cc618e30bcf878313))
* Keto Stack Included in Docker Compose file - Auth ([7be883f](https://github.com/LerianStudio/midaz/commit/7be883fb0a7851d6798eeadfbc79938d20ba4129))


### Bug Fixes

* add comments :bug: ([dfd765f](https://github.com/LerianStudio/midaz/commit/dfd765fab6f1c860879e096eeb2f9527e998d820))

## [1.6.0](https://github.com/LerianStudio/midaz/compare/v1.5.0...v1.6.0) (2024-06-05)


### Bug Fixes

* validate fields parentAccountId and parentOrganizationId that can receive null or check value is an uuid string :bug: ([37648ef](https://github.com/LerianStudio/midaz/commit/37648ef363d50d4baf36d9244f9e7f2417ebe040))

## [1.6.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.5.0...v1.6.0-beta.1) (2024-06-05)


### Bug Fixes

* validate fields parentAccountId and parentOrganizationId that can receive null or check value is an uuid string :bug: ([37648ef](https://github.com/LerianStudio/midaz/commit/37648ef363d50d4baf36d9244f9e7f2417ebe040))

## [1.5.0](https://github.com/LerianStudio/midaz/compare/v1.4.0...v1.5.0) (2024-06-04)


### Bug Fixes

* bring back omitempty on metadata in field _id because cant generate automatic id without :bug: ([d68be08](https://github.com/LerianStudio/midaz/commit/d68be08765d57c7c01d4a9b1f0466070007839c2))

## [1.5.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.4.0...v1.5.0-beta.1) (2024-06-04)


### Bug Fixes

* bring back omitempty on metadata in field _id because cant generate automatic id without :bug: ([d68be08](https://github.com/LerianStudio/midaz/commit/d68be08765d57c7c01d4a9b1f0466070007839c2))

## [1.4.0](https://github.com/LerianStudio/midaz/compare/v1.3.0...v1.4.0) (2024-06-04)

## [1.4.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.3.0...v1.4.0-beta.1) (2024-06-04)

## [1.3.0](https://github.com/LerianStudio/midaz/compare/v1.2.0...v1.3.0) (2024-06-03)


### Features

* add antlr4 in go mod and update to 1.22 :sparkles: ([81ae7bb](https://github.com/LerianStudio/midaz/commit/81ae7bb6e0353a5a3df48a0022a32d49991c8c62))
* add func to extract and validate parameters :sparkles: ([fab06d1](https://github.com/LerianStudio/midaz/commit/fab06d1d299477d765884d6b2f64cb6d49819cef))
* add implementation to paginate organization in postgresql only :sparkles: ([33f9b0a](https://github.com/LerianStudio/midaz/commit/33f9b0a3e4ff8ca559e5180bd9fbf458c65cc2fe))
* add make all-services that can run all services in the makefile :sparkles: ([20637eb](https://github.com/LerianStudio/midaz/commit/20637eb3e50eaa00b8c58ef4bf4dea4d2deb8a2b))
* add migration to create extension "uuid-ossp" on schema public :sparkles: ([fceb8b0](https://github.com/LerianStudio/midaz/commit/fceb8b00f49b57dc95d28f1df507f2333bfa7521))
* add pagination instrument postgresql :sparkles: ([2427093](https://github.com/LerianStudio/midaz/commit/24270935db8b24499d441751cdb94ef606bc8532))
* add pagination ledger postgresql :sparkles: ([a96fe64](https://github.com/LerianStudio/midaz/commit/a96fe64160ba03e6fec57475f8fec1ef44fcd95c))
* add pagination portfolio postgresql :sparkles: ([3f57b98](https://github.com/LerianStudio/midaz/commit/3f57b98c34a63d6daae256dfe781086902c9e81b))
* add pagination response :sparkles: ([b1221c9](https://github.com/LerianStudio/midaz/commit/b1221c94fefd038dfd850ca45d0a8a097c7d4c53))
* add pagination to account only postgresql :sparkles: ([86d4a73](https://github.com/LerianStudio/midaz/commit/86d4a73d4b9ec7f026f929b4cede5ec026de3343))
* add pagination to metadata :sparkles: ([5b09efe](https://github.com/LerianStudio/midaz/commit/5b09efebeaa5409f6d0f36a72acee5814c6bc833))
* add pagination to metadata accounts :sparkles: ([2c23e95](https://github.com/LerianStudio/midaz/commit/2c23e95c29b3c885f70eb1c2ad419a064ad4b448))
* add pagination to metadata instrument :sparkles: ([7c9b344](https://github.com/LerianStudio/midaz/commit/7c9b3449b404616a95c57d136355fed80b3d2c71))
* add pagination to metadata ledger :sparkles: ([421a473](https://github.com/LerianStudio/midaz/commit/421a4736532daffbee10168113143d3263f0939e))
* add pagination to metadata mock and tests :sparkles: ([e97efa7](https://github.com/LerianStudio/midaz/commit/e97efa71c928e8583fa92961053fa713f9fb9e0d))
* add pagination to metadata organization :sparkles: ([7388b29](https://github.com/LerianStudio/midaz/commit/7388b296adefb9e288cfe9252d3c0b20dbc27931))
* add pagination to metadata portfolios :sparkles: ([47c4e15](https://github.com/LerianStudio/midaz/commit/47c4e15f4b701a7b9da9dbc33b2f216fc08763b0))
* add pagination to metadata products :sparkles: ([3cfea5c](https://github.com/LerianStudio/midaz/commit/3cfea5cc996661d7e912e7b5540acdd4defe2fa0))
* add pagination to product, only postgresql :sparkles: ([eb0f981](https://github.com/LerianStudio/midaz/commit/eb0f9818dd25a6ec676a0556c7df7af80e1afb46))
* add readme to show antlr and trillian in transaction :sparkles: ([3c12b13](https://github.com/LerianStudio/midaz/commit/3c12b133dc90aee4275944b421bee661d6b9e363))
* add squirrel and update go mod tidy :sparkles: ([e4bdeed](https://github.com/LerianStudio/midaz/commit/e4bdeeddbe9783b086799d59c365105f4dc32c7d))
* add the gold language that use antlr4, with your parser, lexer and listeners into commons :sparkles: ([4855c21](https://github.com/LerianStudio/midaz/commit/4855c2189dfbeaf458ba35476d1216bb6666aeca))
* add transaction to components and update commands into the main make :sparkles: ([40037a3](https://github.com/LerianStudio/midaz/commit/40037a3bb3b19415133ea7cb937fdac1d797d66e))
* add trillina log temper and refact some container names ([f827d96](https://github.com/LerianStudio/midaz/commit/f827d96317884e419c2579472b3929eb14888951))
* create struct generic to pagination :sparkles: ([af48647](https://github.com/LerianStudio/midaz/commit/af48647b3ce1922d6185258489f6f0fdabee58da))
* **transaction:** exemples files for test :sparkles: ([ad65108](https://github.com/LerianStudio/midaz/commit/ad6510803495b9f234a2b92f37bbadd908ca27ba))


### Bug Fixes

* add -d command in docker up :bug: ([c9dc679](https://github.com/LerianStudio/midaz/commit/c9dc6797b24bb5915826670330b862d39cb250db))
* add and change fields allowSending and allowReceiving on portfolio and accounts :bug: ([eeba628](https://github.com/LerianStudio/midaz/commit/eeba628b1f749e7dbbcb3e662d92dbf7f6208a5a))
* add container_name on ledger docker-compose.yml :bug: ([8f7e028](https://github.com/LerianStudio/midaz/commit/8f7e02826d104580835603b7d8edc6be1d4662f1))
* add in string utils regex features like, ignore accents... :bug: ([a80a698](https://github.com/LerianStudio/midaz/commit/a80a698b76375f809ab98b503fda72396ccb9744))
* adjust method findAll to paginate using keyset and squirrel (not finished) :bug: ([8f4883b](https://github.com/LerianStudio/midaz/commit/8f4883b525bb4c88d3aebad0464ce7d27e6177f0))
* adjust migration to id always be not null and use uuid_generate_v4() as default :bug: ([ea2aaa7](https://github.com/LerianStudio/midaz/commit/ea2aaa77a8ecc5e4a502b2d6fcf4d3d97af112f0))
* adjust query cqrs for use new method signature :bug: ([d87cc5e](https://github.com/LerianStudio/midaz/commit/d87cc5ebc042c8e22fdaa5f78fd321b558f6b9ff))
* change of place the fields allow_sending and allow_receiving :bug: ([3be0010](https://github.com/LerianStudio/midaz/commit/3be0010cd92310a5e79d4fe6f876aa3053a5555d))
* domain adjust interface with new signature method :bug: ([8ea6940](https://github.com/LerianStudio/midaz/commit/8ea6940ee4a3300eb3a247fde238e1c850bb27fc))
* golang lint mess imports :bug: ([8a40f2b](https://github.com/LerianStudio/midaz/commit/8a40f2bc64a68233c4b55523357062b3741207b6))
* interface signature for organization :bug: ([cb5df35](https://github.com/LerianStudio/midaz/commit/cb5df3529da50ecbe89c9ffa4333029e083b5caf))
* make lint :bug: ([0281101](https://github.com/LerianStudio/midaz/commit/0281101e99125b103eacafb07f6549137a099bae))
* make lint :bug: ([660698b](https://github.com/LerianStudio/midaz/commit/660698bec3e15616f2c29444c4910542e4e18782))
* make sec, lint and tests :bug: ([f10fa90](https://github.com/LerianStudio/midaz/commit/f10fa90e5b7491308e18fadbd2efeb43224c9c1c))
* makefiles adjust commands and logs :bug: ([f5859e3](https://github.com/LerianStudio/midaz/commit/f5859e31ad557b82ce9b0e9346a213e6c3bc75a1))
* passing field metadata to instrument :bug: ([87d10c8](https://github.com/LerianStudio/midaz/commit/87d10c8f9f75a593491d4e0843962653b72c069a))
* passing field metadata to portfolio :bug: ([5356e5c](https://github.com/LerianStudio/midaz/commit/5356e5cb22a9957b0c3cff0d0e52a539a2cc7187))
* ports adjust headers :bug: ([97dc2eb](https://github.com/LerianStudio/midaz/commit/97dc2eb660d3369082e950dd338a4a0ac4bffd32))
* regenerated mock :bug: ([5383978](https://github.com/LerianStudio/midaz/commit/538397890c5542b311c0cf7df94fa3b8a073dab8))
* remove duplicated currency :bug: ([38b1b8b](https://github.com/LerianStudio/midaz/commit/38b1b8bb7e1c6a138ade8e666d6856d595363a37))
* remove pagination from  organization struct to a separated object generic :bug: ([0cc066d](https://github.com/LerianStudio/midaz/commit/0cc066d362104f70775efdb1b3a7b74a6cbd4453))
* remove squirrel :bug: ([941ded6](https://github.com/LerianStudio/midaz/commit/941ded618a426a4c54a8937234f0f7fa22708def))
* remove unusable features from mpostgres :bug: ([0e0c090](https://github.com/LerianStudio/midaz/commit/0e0c090850e2cca7dbd20bcff3e9aa0e58eafef0))
* remove wrong file auto generated :bug: ([67533b7](https://github.com/LerianStudio/midaz/commit/67533b76f61d8d2683ca155d095309931dd4ca5a))
* return squirrel :bug: ([7b7c301](https://github.com/LerianStudio/midaz/commit/7b7c30145d24fcca97195b190b448d6b18f1a54a))
* some adjusts on query header strutc :bug: ([adb03ea](https://github.com/LerianStudio/midaz/commit/adb03eaeb6597009734565967d977a093765f6cd))
* update lib zitadel oidc v2 to v3 :bug: ([1638894](https://github.com/LerianStudio/midaz/commit/1638894d8765da59efb5bfbaf31b337d005538aa))
* **cwe-406:** update lib zitadel oidc v2 to v3 and update some code to non retro compatibility :bug: ([3053f08](https://github.com/LerianStudio/midaz/commit/3053f087bcab2c21535b97814c0ce89899ee05e6))
* updated postman :bug: ([750bd62](https://github.com/LerianStudio/midaz/commit/750bd620f8a682e4670707a353bed0aa4eb82a9c))

## [1.3.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.2.0...v1.3.0-beta.1) (2024-06-03)


### Features

* add antlr4 in go mod and update to 1.22 :sparkles: ([81ae7bb](https://github.com/LerianStudio/midaz/commit/81ae7bb6e0353a5a3df48a0022a32d49991c8c62))
* add func to extract and validate parameters :sparkles: ([fab06d1](https://github.com/LerianStudio/midaz/commit/fab06d1d299477d765884d6b2f64cb6d49819cef))
* add implementation to paginate organization in postgresql only :sparkles: ([33f9b0a](https://github.com/LerianStudio/midaz/commit/33f9b0a3e4ff8ca559e5180bd9fbf458c65cc2fe))
* add make all-services that can run all services in the makefile :sparkles: ([20637eb](https://github.com/LerianStudio/midaz/commit/20637eb3e50eaa00b8c58ef4bf4dea4d2deb8a2b))
* add migration to create extension "uuid-ossp" on schema public :sparkles: ([fceb8b0](https://github.com/LerianStudio/midaz/commit/fceb8b00f49b57dc95d28f1df507f2333bfa7521))
* add pagination instrument postgresql :sparkles: ([2427093](https://github.com/LerianStudio/midaz/commit/24270935db8b24499d441751cdb94ef606bc8532))
* add pagination ledger postgresql :sparkles: ([a96fe64](https://github.com/LerianStudio/midaz/commit/a96fe64160ba03e6fec57475f8fec1ef44fcd95c))
* add pagination portfolio postgresql :sparkles: ([3f57b98](https://github.com/LerianStudio/midaz/commit/3f57b98c34a63d6daae256dfe781086902c9e81b))
* add pagination response :sparkles: ([b1221c9](https://github.com/LerianStudio/midaz/commit/b1221c94fefd038dfd850ca45d0a8a097c7d4c53))
* add pagination to account only postgresql :sparkles: ([86d4a73](https://github.com/LerianStudio/midaz/commit/86d4a73d4b9ec7f026f929b4cede5ec026de3343))
* add pagination to metadata :sparkles: ([5b09efe](https://github.com/LerianStudio/midaz/commit/5b09efebeaa5409f6d0f36a72acee5814c6bc833))
* add pagination to metadata accounts :sparkles: ([2c23e95](https://github.com/LerianStudio/midaz/commit/2c23e95c29b3c885f70eb1c2ad419a064ad4b448))
* add pagination to metadata instrument :sparkles: ([7c9b344](https://github.com/LerianStudio/midaz/commit/7c9b3449b404616a95c57d136355fed80b3d2c71))
* add pagination to metadata ledger :sparkles: ([421a473](https://github.com/LerianStudio/midaz/commit/421a4736532daffbee10168113143d3263f0939e))
* add pagination to metadata mock and tests :sparkles: ([e97efa7](https://github.com/LerianStudio/midaz/commit/e97efa71c928e8583fa92961053fa713f9fb9e0d))
* add pagination to metadata organization :sparkles: ([7388b29](https://github.com/LerianStudio/midaz/commit/7388b296adefb9e288cfe9252d3c0b20dbc27931))
* add pagination to metadata portfolios :sparkles: ([47c4e15](https://github.com/LerianStudio/midaz/commit/47c4e15f4b701a7b9da9dbc33b2f216fc08763b0))
* add pagination to metadata products :sparkles: ([3cfea5c](https://github.com/LerianStudio/midaz/commit/3cfea5cc996661d7e912e7b5540acdd4defe2fa0))
* add pagination to product, only postgresql :sparkles: ([eb0f981](https://github.com/LerianStudio/midaz/commit/eb0f9818dd25a6ec676a0556c7df7af80e1afb46))
* add readme to show antlr and trillian in transaction :sparkles: ([3c12b13](https://github.com/LerianStudio/midaz/commit/3c12b133dc90aee4275944b421bee661d6b9e363))
* add squirrel and update go mod tidy :sparkles: ([e4bdeed](https://github.com/LerianStudio/midaz/commit/e4bdeeddbe9783b086799d59c365105f4dc32c7d))
* add the gold language that use antlr4, with your parser, lexer and listeners into commons :sparkles: ([4855c21](https://github.com/LerianStudio/midaz/commit/4855c2189dfbeaf458ba35476d1216bb6666aeca))
* add transaction to components and update commands into the main make :sparkles: ([40037a3](https://github.com/LerianStudio/midaz/commit/40037a3bb3b19415133ea7cb937fdac1d797d66e))
* add trillina log temper and refact some container names ([f827d96](https://github.com/LerianStudio/midaz/commit/f827d96317884e419c2579472b3929eb14888951))
* create struct generic to pagination :sparkles: ([af48647](https://github.com/LerianStudio/midaz/commit/af48647b3ce1922d6185258489f6f0fdabee58da))
* **transaction:** exemples files for test :sparkles: ([ad65108](https://github.com/LerianStudio/midaz/commit/ad6510803495b9f234a2b92f37bbadd908ca27ba))


### Bug Fixes

* add -d command in docker up :bug: ([c9dc679](https://github.com/LerianStudio/midaz/commit/c9dc6797b24bb5915826670330b862d39cb250db))
* add and change fields allowSending and allowReceiving on portfolio and accounts :bug: ([eeba628](https://github.com/LerianStudio/midaz/commit/eeba628b1f749e7dbbcb3e662d92dbf7f6208a5a))
* add container_name on ledger docker-compose.yml :bug: ([8f7e028](https://github.com/LerianStudio/midaz/commit/8f7e02826d104580835603b7d8edc6be1d4662f1))
* add in string utils regex features like, ignore accents... :bug: ([a80a698](https://github.com/LerianStudio/midaz/commit/a80a698b76375f809ab98b503fda72396ccb9744))
* adjust method findAll to paginate using keyset and squirrel (not finished) :bug: ([8f4883b](https://github.com/LerianStudio/midaz/commit/8f4883b525bb4c88d3aebad0464ce7d27e6177f0))
* adjust migration to id always be not null and use uuid_generate_v4() as default :bug: ([ea2aaa7](https://github.com/LerianStudio/midaz/commit/ea2aaa77a8ecc5e4a502b2d6fcf4d3d97af112f0))
* adjust query cqrs for use new method signature :bug: ([d87cc5e](https://github.com/LerianStudio/midaz/commit/d87cc5ebc042c8e22fdaa5f78fd321b558f6b9ff))
* change of place the fields allow_sending and allow_receiving :bug: ([3be0010](https://github.com/LerianStudio/midaz/commit/3be0010cd92310a5e79d4fe6f876aa3053a5555d))
* domain adjust interface with new signature method :bug: ([8ea6940](https://github.com/LerianStudio/midaz/commit/8ea6940ee4a3300eb3a247fde238e1c850bb27fc))
* golang lint mess imports :bug: ([8a40f2b](https://github.com/LerianStudio/midaz/commit/8a40f2bc64a68233c4b55523357062b3741207b6))
* interface signature for organization :bug: ([cb5df35](https://github.com/LerianStudio/midaz/commit/cb5df3529da50ecbe89c9ffa4333029e083b5caf))
* make lint :bug: ([0281101](https://github.com/LerianStudio/midaz/commit/0281101e99125b103eacafb07f6549137a099bae))
* make lint :bug: ([660698b](https://github.com/LerianStudio/midaz/commit/660698bec3e15616f2c29444c4910542e4e18782))
* make sec, lint and tests :bug: ([f10fa90](https://github.com/LerianStudio/midaz/commit/f10fa90e5b7491308e18fadbd2efeb43224c9c1c))
* makefiles adjust commands and logs :bug: ([f5859e3](https://github.com/LerianStudio/midaz/commit/f5859e31ad557b82ce9b0e9346a213e6c3bc75a1))
* passing field metadata to instrument :bug: ([87d10c8](https://github.com/LerianStudio/midaz/commit/87d10c8f9f75a593491d4e0843962653b72c069a))
* passing field metadata to portfolio :bug: ([5356e5c](https://github.com/LerianStudio/midaz/commit/5356e5cb22a9957b0c3cff0d0e52a539a2cc7187))
* ports adjust headers :bug: ([97dc2eb](https://github.com/LerianStudio/midaz/commit/97dc2eb660d3369082e950dd338a4a0ac4bffd32))
* regenerated mock :bug: ([5383978](https://github.com/LerianStudio/midaz/commit/538397890c5542b311c0cf7df94fa3b8a073dab8))
* remove duplicated currency :bug: ([38b1b8b](https://github.com/LerianStudio/midaz/commit/38b1b8bb7e1c6a138ade8e666d6856d595363a37))
* remove pagination from  organization struct to a separated object generic :bug: ([0cc066d](https://github.com/LerianStudio/midaz/commit/0cc066d362104f70775efdb1b3a7b74a6cbd4453))
* remove squirrel :bug: ([941ded6](https://github.com/LerianStudio/midaz/commit/941ded618a426a4c54a8937234f0f7fa22708def))
* remove unusable features from mpostgres :bug: ([0e0c090](https://github.com/LerianStudio/midaz/commit/0e0c090850e2cca7dbd20bcff3e9aa0e58eafef0))
* remove wrong file auto generated :bug: ([67533b7](https://github.com/LerianStudio/midaz/commit/67533b76f61d8d2683ca155d095309931dd4ca5a))
* return squirrel :bug: ([7b7c301](https://github.com/LerianStudio/midaz/commit/7b7c30145d24fcca97195b190b448d6b18f1a54a))
* some adjusts on query header strutc :bug: ([adb03ea](https://github.com/LerianStudio/midaz/commit/adb03eaeb6597009734565967d977a093765f6cd))
* update lib zitadel oidc v2 to v3 :bug: ([1638894](https://github.com/LerianStudio/midaz/commit/1638894d8765da59efb5bfbaf31b337d005538aa))
* **cwe-406:** update lib zitadel oidc v2 to v3 and update some code to non retro compatibility :bug: ([3053f08](https://github.com/LerianStudio/midaz/commit/3053f087bcab2c21535b97814c0ce89899ee05e6))
* updated postman :bug: ([750bd62](https://github.com/LerianStudio/midaz/commit/750bd620f8a682e4670707a353bed0aa4eb82a9c))

## [1.2.0](https://github.com/LerianStudio/midaz/compare/v1.1.0...v1.2.0) (2024-05-23)


### Bug Fixes

* fix patch updates to accept only specific fields, not all like put :bug: ([95c2847](https://github.com/LerianStudio/midaz/commit/95c284760b82e0ed3d173ed83728dc03417dc3a5))
* remove not null from field entity_id in account :bug: ([921b21e](https://github.com/LerianStudio/midaz/commit/921b21ef6bc4c7c9ddb957f48b3849a93c9551ee))

## [1.1.0](https://github.com/LerianStudio/midaz/compare/v1.0.0...v1.1.0) (2024-05-21)


### Features

* business message :sparkles: ([c6e3c97](https://github.com/LerianStudio/midaz/commit/c6e3c979edfd578d61f88525360d771336be7da8))
* create method that search instrument by name or code to cant insert again ([8e01080](https://github.com/LerianStudio/midaz/commit/8e01080e7a44656568b66aed0bfeee6dc6b336a7))
* create new method findbyalias :sparkles: ([6d86734](https://github.com/LerianStudio/midaz/commit/6d867340c58251cb45f13c08b89124187cb1e8f7))
* create two methods, validate type and validate currency validate ISO 4217 :bug: ([09c622b](https://github.com/LerianStudio/midaz/commit/09c622b908989bd334fab244e3639f312ca1b0df))
* re run mock :sparkles: ([5cd0b70](https://github.com/LerianStudio/midaz/commit/5cd0b7002a7fb416cf7a316cb050a565afa17182))


### Bug Fixes

* (cqrs): remove delete metadata when update object with field is null ([9142901](https://github.com/LerianStudio/midaz/commit/91429013d88bbfc5183487284bde8f11a4f00297))
* adjust make lint ([dacca62](https://github.com/LerianStudio/midaz/commit/dacca62bfcb272c9d70c10de95fdd4473d3b97c2))
* adjust path mock to generate new files and add new method interface in instrument :bug: ([ecbfce9](https://github.com/LerianStudio/midaz/commit/ecbfce9b4d74dbbb72df384c1f697c9ff9a8772e))
* ajust alias to receive nil :bug: ([19844fd](https://github.com/LerianStudio/midaz/commit/19844fdc8a507ac1060812630419c495cb7bf326))
* bugs and new implements features :bug: ([8b8ee76](https://github.com/LerianStudio/midaz/commit/8b8ee76dfd7a2d7c446eab205b627ddf1c87b622))
* business message :bug: ([d3c35d7](https://github.com/LerianStudio/midaz/commit/d3c35d7da834698a2b50e59e16db519132b8786b))
* create method to validate if code has letter uppercase :bug: ([36f6c0e](https://github.com/LerianStudio/midaz/commit/36f6c0e295f24a809acde2332d4b6c3b51eefd8b))
* env default local :bug: ([b1d8f04](https://github.com/LerianStudio/midaz/commit/b1d8f0492c7cdd0bc2828b55d5f632f1c2694adc))
* golint :bug: ([481e1fe](https://github.com/LerianStudio/midaz/commit/481e1fec585ad094dafccb0b4a4e0dc4df600f7c))
* lint :bug: ([9508657](https://github.com/LerianStudio/midaz/commit/950865748e3fdcf340599c92cd3143ffc737f87f))
* lint and error message :bug: ([be8637e](https://github.com/LerianStudio/midaz/commit/be8637eb10a2ec105a6da841eae56d7ac7b0827d))
* migration alias to receive null :bug: ([9c83a9c](https://github.com/LerianStudio/midaz/commit/9c83a9ccb693031b588a67e5f42b03cc5b26a509))
* regenerate mocks :bug: ([8592e17](https://github.com/LerianStudio/midaz/commit/8592e17ab449151972af43ebc64d6dfdc9975087))
* remove and update postman :bug: ([0971d13](https://github.com/LerianStudio/midaz/commit/0971d133c9ea969d9c063e8acb7a617edb620be2))
* remove json unmarshal from status in method find and findall ([021e5af](https://github.com/LerianStudio/midaz/commit/021e5af12b8ff6791bac9c694e5de157efbad4c7))
* removes omitempty to return field even than null :bug: ([030ea64](https://github.com/LerianStudio/midaz/commit/030ea6406baf1a5ced486e4b3b2ab577f44adedf))
* **ledger:** when string ParentOrganizationID is empty set nil ([6f6c044](https://github.com/LerianStudio/midaz/commit/6f6c0449c0833c333d06aeabcfeeeee1108c0256))

## [1.1.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.0.0...v1.1.0-beta.1) (2024-05-21)


### Features

* business message :sparkles: ([c6e3c97](https://github.com/LerianStudio/midaz/commit/c6e3c979edfd578d61f88525360d771336be7da8))
* create method that search instrument by name or code to cant insert again ([8e01080](https://github.com/LerianStudio/midaz/commit/8e01080e7a44656568b66aed0bfeee6dc6b336a7))
* create new method findbyalias :sparkles: ([6d86734](https://github.com/LerianStudio/midaz/commit/6d867340c58251cb45f13c08b89124187cb1e8f7))
* create two methods, validate type and validate currency validate ISO 4217 :bug: ([09c622b](https://github.com/LerianStudio/midaz/commit/09c622b908989bd334fab244e3639f312ca1b0df))
* re run mock :sparkles: ([5cd0b70](https://github.com/LerianStudio/midaz/commit/5cd0b7002a7fb416cf7a316cb050a565afa17182))


### Bug Fixes

* (cqrs): remove delete metadata when update object with field is null ([9142901](https://github.com/LerianStudio/midaz/commit/91429013d88bbfc5183487284bde8f11a4f00297))
* adjust make lint ([dacca62](https://github.com/LerianStudio/midaz/commit/dacca62bfcb272c9d70c10de95fdd4473d3b97c2))
* adjust path mock to generate new files and add new method interface in instrument :bug: ([ecbfce9](https://github.com/LerianStudio/midaz/commit/ecbfce9b4d74dbbb72df384c1f697c9ff9a8772e))
* ajust alias to receive nil :bug: ([19844fd](https://github.com/LerianStudio/midaz/commit/19844fdc8a507ac1060812630419c495cb7bf326))
* bugs and new implements features :bug: ([8b8ee76](https://github.com/LerianStudio/midaz/commit/8b8ee76dfd7a2d7c446eab205b627ddf1c87b622))
* business message :bug: ([d3c35d7](https://github.com/LerianStudio/midaz/commit/d3c35d7da834698a2b50e59e16db519132b8786b))
* create method to validate if code has letter uppercase :bug: ([36f6c0e](https://github.com/LerianStudio/midaz/commit/36f6c0e295f24a809acde2332d4b6c3b51eefd8b))
* env default local :bug: ([b1d8f04](https://github.com/LerianStudio/midaz/commit/b1d8f0492c7cdd0bc2828b55d5f632f1c2694adc))
* golint :bug: ([481e1fe](https://github.com/LerianStudio/midaz/commit/481e1fec585ad094dafccb0b4a4e0dc4df600f7c))
* lint :bug: ([9508657](https://github.com/LerianStudio/midaz/commit/950865748e3fdcf340599c92cd3143ffc737f87f))
* lint and error message :bug: ([be8637e](https://github.com/LerianStudio/midaz/commit/be8637eb10a2ec105a6da841eae56d7ac7b0827d))
* migration alias to receive null :bug: ([9c83a9c](https://github.com/LerianStudio/midaz/commit/9c83a9ccb693031b588a67e5f42b03cc5b26a509))
* regenerate mocks :bug: ([8592e17](https://github.com/LerianStudio/midaz/commit/8592e17ab449151972af43ebc64d6dfdc9975087))
* remove and update postman :bug: ([0971d13](https://github.com/LerianStudio/midaz/commit/0971d133c9ea969d9c063e8acb7a617edb620be2))
* remove json unmarshal from status in method find and findall ([021e5af](https://github.com/LerianStudio/midaz/commit/021e5af12b8ff6791bac9c694e5de157efbad4c7))
* removes omitempty to return field even than null :bug: ([030ea64](https://github.com/LerianStudio/midaz/commit/030ea6406baf1a5ced486e4b3b2ab577f44adedf))
* **ledger:** when string ParentOrganizationID is empty set nil ([6f6c044](https://github.com/LerianStudio/midaz/commit/6f6c0449c0833c333d06aeabcfeeeee1108c0256))

## 1.0.0 (2024-05-17)


### Features

* Open tech for all ([cd4cf48](https://github.com/LerianStudio/midaz/commit/cd4cf4874503756b6b051723f512fde41323e609))


### Bug Fixes

* change conversion of a signed 64-bit integer to int ([2fd77c2](https://github.com/LerianStudio/midaz/commit/2fd77c298a1aa4c74dbfa5e030ec65ca3628afd4))

## [1.0.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.0.0-beta.1...v1.0.0-beta.2) (2024-05-17)


### Bug Fixes

* change conversion of a signed 64-bit integer to int ([2fd77c2](https://github.com/LerianStudio/midaz/commit/2fd77c298a1aa4c74dbfa5e030ec65ca3628afd4))

## 1.0.0-beta.1 (2024-05-17)


### Features

* Open tech for all ([cd4cf48](https://github.com/LerianStudio/midaz/commit/cd4cf4874503756b6b051723f512fde41323e609))

## [1.17.0](https://github.com/LerianStudio/midaz-private/compare/v1.16.0...v1.17.0) (2024-05-17)


### Features

* enable CodeQL and adjust Readme :sparkles: ([7037bba](https://github.com/LerianStudio/midaz-private/commit/7037bba5a16d8e96d15e56f9f0b137524ed17a14))


### Bug Fixes

* clint :bug: ([9953ad5](https://github.com/LerianStudio/midaz-private/commit/9953ad58e904bf0d30bac70389f880c690a77b6d))
* source and imports :bug: ([b91ec61](https://github.com/LerianStudio/midaz-private/commit/b91ec61193be7e2a0d78ae8f2047e90335c434e5))

## [1.17.0-beta.1](https://github.com/LerianStudio/midaz-private/compare/v1.16.0...v1.17.0-beta.1) (2024-05-17)


### Features

* enable CodeQL and adjust Readme :sparkles: ([7037bba](https://github.com/LerianStudio/midaz-private/commit/7037bba5a16d8e96d15e56f9f0b137524ed17a14))


### Bug Fixes

* clint :bug: ([9953ad5](https://github.com/LerianStudio/midaz-private/commit/9953ad58e904bf0d30bac70389f880c690a77b6d))
* source and imports :bug: ([b91ec61](https://github.com/LerianStudio/midaz-private/commit/b91ec61193be7e2a0d78ae8f2047e90335c434e5))

## [1.16.0](https://github.com/LerianStudio/midaz-private/compare/v1.15.0...v1.16.0) (2024-05-17)

## [1.15.0](https://github.com/LerianStudio/midaz-private/compare/v1.14.0...v1.15.0) (2024-05-13)


### Bug Fixes

* adapters :bug: ([f1eab22](https://github.com/LerianStudio/midaz-private/commit/f1eab221117afc8b4f132eb75c2485f034de68aa))
* domain :bug: ([f066eec](https://github.com/LerianStudio/midaz-private/commit/f066eec4d497fac2bde509e81315f8f11027ff6c))
* final :bug: ([3071ab2](https://github.com/LerianStudio/midaz-private/commit/3071ab246cbb085f2df438664f1440b385723ad9))
* gen :bug: ([dd601a5](https://github.com/LerianStudio/midaz-private/commit/dd601a59dec321d9b98c2cad08fa94b8b505c42a))
* import :bug: ([d66ffae](https://github.com/LerianStudio/midaz-private/commit/d66ffae65b0ebc14cbef4c746f3d047a5e3bca5b))
* imports :bug: ([b4649ec](https://github.com/LerianStudio/midaz-private/commit/b4649ecb2824fc7ce4d94c01a6cd6e393a5ed910))
* metadata :bug: ([a15b08e](https://github.com/LerianStudio/midaz-private/commit/a15b08e1004cfa69532ed9d080bf9d91b6a8740d))
* routes :bug: ([340ebf3](https://github.com/LerianStudio/midaz-private/commit/340ebf39106236c2fc134fa079243b526ba7093f))

## [1.15.0-beta.1](https://github.com/LerianStudio/midaz-private/compare/v1.14.0...v1.15.0-beta.1) (2024-05-13)


### Bug Fixes

* adapters :bug: ([f1eab22](https://github.com/LerianStudio/midaz-private/commit/f1eab221117afc8b4f132eb75c2485f034de68aa))
* domain :bug: ([f066eec](https://github.com/LerianStudio/midaz-private/commit/f066eec4d497fac2bde509e81315f8f11027ff6c))
* final :bug: ([3071ab2](https://github.com/LerianStudio/midaz-private/commit/3071ab246cbb085f2df438664f1440b385723ad9))
* gen :bug: ([dd601a5](https://github.com/LerianStudio/midaz-private/commit/dd601a59dec321d9b98c2cad08fa94b8b505c42a))
* import :bug: ([d66ffae](https://github.com/LerianStudio/midaz-private/commit/d66ffae65b0ebc14cbef4c746f3d047a5e3bca5b))
* imports :bug: ([b4649ec](https://github.com/LerianStudio/midaz-private/commit/b4649ecb2824fc7ce4d94c01a6cd6e393a5ed910))
* metadata :bug: ([a15b08e](https://github.com/LerianStudio/midaz-private/commit/a15b08e1004cfa69532ed9d080bf9d91b6a8740d))
* routes :bug: ([340ebf3](https://github.com/LerianStudio/midaz-private/commit/340ebf39106236c2fc134fa079243b526ba7093f))

## [1.14.0](https://github.com/LerianStudio/midaz-private/compare/v1.13.0...v1.14.0) (2024-05-10)


### Bug Fixes

* get connection everytime and mongo database name :bug: ([36e9ffa](https://github.com/LerianStudio/midaz-private/commit/36e9ffa586a1dbca8c043d3eaa0ac80f34d431b4))

## [1.14.0-beta.1](https://github.com/LerianStudio/midaz-private/compare/v1.13.0...v1.14.0-beta.1) (2024-05-10)


### Bug Fixes

* get connection everytime and mongo database name :bug: ([36e9ffa](https://github.com/LerianStudio/midaz-private/commit/36e9ffa586a1dbca8c043d3eaa0ac80f34d431b4))

## [1.13.0](https://github.com/LerianStudio/midaz-private/compare/v1.12.0...v1.13.0) (2024-05-10)


### Bug Fixes

* gen :bug: ([d196ebb](https://github.com/LerianStudio/midaz-private/commit/d196ebb742ac9a7df39f6224ace0bbcdd17a1a4b))
* make lint :bug: ([b89f0f4](https://github.com/LerianStudio/midaz-private/commit/b89f0f4eaa8067fa339b855012f10557ce68faa3))
* make lint and make formmat :bug: ([c559f01](https://github.com/LerianStudio/midaz-private/commit/c559f012b9e4a2ba60d6e2acffd06cceba9f9893))
* remove docker-composer version and make lint :bug: ([b002f0b](https://github.com/LerianStudio/midaz-private/commit/b002f0be0e1cb8ee17661855c55549ad275b20ff))

## [1.13.0-beta.1](https://github.com/LerianStudio/midaz-private/compare/v1.12.0...v1.13.0-beta.1) (2024-05-10)


### Bug Fixes

* gen :bug: ([d196ebb](https://github.com/LerianStudio/midaz-private/commit/d196ebb742ac9a7df39f6224ace0bbcdd17a1a4b))
* make lint :bug: ([b89f0f4](https://github.com/LerianStudio/midaz-private/commit/b89f0f4eaa8067fa339b855012f10557ce68faa3))
* make lint and make formmat :bug: ([c559f01](https://github.com/LerianStudio/midaz-private/commit/c559f012b9e4a2ba60d6e2acffd06cceba9f9893))
* remove docker-composer version and make lint :bug: ([b002f0b](https://github.com/LerianStudio/midaz-private/commit/b002f0be0e1cb8ee17661855c55549ad275b20ff))

## [1.12.0](https://github.com/LerianStudio/midaz-private/compare/v1.11.0...v1.12.0) (2024-05-09)


### Bug Fixes

* adapters :bug: ([6ca68a5](https://github.com/LerianStudio/midaz-private/commit/6ca68a59c203da4448cff46c33221a1c6666a168))
* adapters :bug: ([34f3944](https://github.com/LerianStudio/midaz-private/commit/34f39444aba0027e8ae3afc0b10ee09b4f812b49))
* command tests :bug: ([4ccd163](https://github.com/LerianStudio/midaz-private/commit/4ccd163e39f2c292b4952ba4df5531626684b7c8))
* domain :bug: ([5742d35](https://github.com/LerianStudio/midaz-private/commit/5742d353bddf58c9afd11303918c5d44574b8ae5))
* make lint :bug: ([cbbc9bb](https://github.com/LerianStudio/midaz-private/commit/cbbc9bbe324482c01d59f58d1c9f2793392c539f))
* migrations :bug: ([7120e4c](https://github.com/LerianStudio/midaz-private/commit/7120e4c7c7012e06e8ffdbc708bbe185863fb1f7))
* mock :bug: ([62a08fd](https://github.com/LerianStudio/midaz-private/commit/62a08fdd401f13b3d4a13d047253dde315537a8f))
* ports :bug: ([b1142f3](https://github.com/LerianStudio/midaz-private/commit/b1142f3d500c5a6241df681e471172a189ccf105))
* postman :bug: ([ab44d0a](https://github.com/LerianStudio/midaz-private/commit/ab44d0a31b3a4fb41920abedbd64508dfbf65bde))
* query tests :bug: ([c974c5d](https://github.com/LerianStudio/midaz-private/commit/c974c5d8137b6387b86a7f7894c153ac62be12d6))

## [1.11.0](https://github.com/LerianStudio/midaz-private/compare/v1.10.0...v1.11.0) (2024-05-08)


### Features

* Creating parentOrganizationId to Organizations ([b1f7c9f](https://github.com/LerianStudio/midaz-private/commit/b1f7c9fe147d3440cbc896221364a2519329e8fa))


### Bug Fixes

* adapters :bug: ([8735d43](https://github.com/LerianStudio/midaz-private/commit/8735d43e4f5dc05f1ae8ccb0ed087e5761c9501e))
* adapters :bug: ([d763478](https://github.com/LerianStudio/midaz-private/commit/d763478d5a9bb44783e77ab0167340df4445c5ee))
* add version in conventional-changelog-conventionalcommits extra plugin :bug: ([b6d100b](https://github.com/LerianStudio/midaz-private/commit/b6d100b928d18d2a35331a87feca50c779c8447f))
* command :bug: ([97fb718](https://github.com/LerianStudio/midaz-private/commit/97fb718f3725f652746d34434989ba7bf18aaf63))
* command sql ([5cf410f](https://github.com/LerianStudio/midaz-private/commit/5cf410fc6b7eef63698ff4cbc3c48eea7651b3e4))
* commands :bug: ([eb2eda0](https://github.com/LerianStudio/midaz-private/commit/eb2eda09212af7c1837a6d1fa6b987a52a9509c6))
* domains :bug: ([3c7a6bd](https://github.com/LerianStudio/midaz-private/commit/3c7a6bd39fd1f9f182242461b44694882243f84e))
* final adjustments ([9ad840e](https://github.com/LerianStudio/midaz-private/commit/9ad840ef0e1e39cad31288cc9b39bbd368d575e0))
* gofmt ([a9f0544](https://github.com/LerianStudio/midaz-private/commit/a9f0544a38508e9d3b37794a1b44af88216c63bb))
* handlers and routes ([98ba8ea](https://github.com/LerianStudio/midaz-private/commit/98ba8eae8369e85727292ba7ecc66c792f9390d3))
* interface and postgres implementation ([ae4fa6f](https://github.com/LerianStudio/midaz-private/commit/ae4fa6ffdba9f5f91c169611eea864e3efb3cb09))
* lint ([23bdd49](https://github.com/LerianStudio/midaz-private/commit/23bdd49daced9c6040134706871a6c7811d267fe))
* make lint, make sec and tests :bug: ([b8df6a4](https://github.com/LerianStudio/midaz-private/commit/b8df6a45ddecf7cd61e5db2a41e2d1cd7ace404d))
* make sec and make lint ([fac8e3a](https://github.com/LerianStudio/midaz-private/commit/fac8e3a392a5139236fd8dab1badb656e7e2fc35))
* migrations ([82c82ba](https://github.com/LerianStudio/midaz-private/commit/82c82ba7b20c966d5dc6de13937c3386b21cc699))
* migrations :bug: ([f5a2ddf](https://github.com/LerianStudio/midaz-private/commit/f5a2ddfcb83558abcda52159af3de36ae0c0bdb3))
* ports :bug: ([96e2b8c](https://github.com/LerianStudio/midaz-private/commit/96e2b8cf800fdb23496804f326799dc0e91c39cd))
* ports :bug: ([37d1010](https://github.com/LerianStudio/midaz-private/commit/37d1010e4bed83b73d993582e137135c048771c7))
* ports :bug: ([4e2664c](https://github.com/LerianStudio/midaz-private/commit/4e2664ce27a48fca5b359a102e685c56bc81be0f))
* postman :bug: ([dd7d9c3](https://github.com/LerianStudio/midaz-private/commit/dd7d9c39c9ff182b2ffd7d4d1ebb274b2492a541))
* queries :bug: ([ecaaa34](https://github.com/LerianStudio/midaz-private/commit/ecaaa34b715aca14159167b26a1a52a16667e884))
* query sql ([fdc2de8](https://github.com/LerianStudio/midaz-private/commit/fdc2de8841246382ed4cf7edf6026990e041f187))
* **divisions:** remove everything from divisions ([5cbed6e](https://github.com/LerianStudio/midaz-private/commit/5cbed6e67ad219ef7aacb4190121f1b6ce804999))
* remove immudb from ledger ([6264110](https://github.com/LerianStudio/midaz-private/commit/6264110af51d4d9d2222d760be91a2983ee4f050))
* template ([5519aa2](https://github.com/LerianStudio/midaz-private/commit/5519aa2d614bb845108b87f967b91c32814040f8))
* tests ([4c3be58](https://github.com/LerianStudio/midaz-private/commit/4c3be58a69a79e2aec1453a93dc7b411388ddac4))

## [1.11.0-beta.3](https://github.com/LerianStudio/midaz-private/compare/v1.11.0-beta.2...v1.11.0-beta.3) (2024-05-08)


### Bug Fixes

* adapters :bug: ([8735d43](https://github.com/LerianStudio/midaz-private/commit/8735d43e4f5dc05f1ae8ccb0ed087e5761c9501e))
* adapters :bug: ([d763478](https://github.com/LerianStudio/midaz-private/commit/d763478d5a9bb44783e77ab0167340df4445c5ee))
* command :bug: ([97fb718](https://github.com/LerianStudio/midaz-private/commit/97fb718f3725f652746d34434989ba7bf18aaf63))
* commands :bug: ([eb2eda0](https://github.com/LerianStudio/midaz-private/commit/eb2eda09212af7c1837a6d1fa6b987a52a9509c6))
* domains :bug: ([3c7a6bd](https://github.com/LerianStudio/midaz-private/commit/3c7a6bd39fd1f9f182242461b44694882243f84e))
* make lint, make sec and tests :bug: ([b8df6a4](https://github.com/LerianStudio/midaz-private/commit/b8df6a45ddecf7cd61e5db2a41e2d1cd7ace404d))
* migrations :bug: ([f5a2ddf](https://github.com/LerianStudio/midaz-private/commit/f5a2ddfcb83558abcda52159af3de36ae0c0bdb3))
* ports :bug: ([96e2b8c](https://github.com/LerianStudio/midaz-private/commit/96e2b8cf800fdb23496804f326799dc0e91c39cd))
* ports :bug: ([37d1010](https://github.com/LerianStudio/midaz-private/commit/37d1010e4bed83b73d993582e137135c048771c7))
* ports :bug: ([4e2664c](https://github.com/LerianStudio/midaz-private/commit/4e2664ce27a48fca5b359a102e685c56bc81be0f))
* postman :bug: ([dd7d9c3](https://github.com/LerianStudio/midaz-private/commit/dd7d9c39c9ff182b2ffd7d4d1ebb274b2492a541))
* queries :bug: ([ecaaa34](https://github.com/LerianStudio/midaz-private/commit/ecaaa34b715aca14159167b26a1a52a16667e884))

## [1.11.0-beta.2](https://github.com/LerianStudio/midaz-private/compare/v1.11.0-beta.1...v1.11.0-beta.2) (2024-05-07)


### Features

* Creating parentOrganizationId to Organizations ([b1f7c9f](https://github.com/LerianStudio/midaz-private/commit/b1f7c9fe147d3440cbc896221364a2519329e8fa))


### Bug Fixes

* add version in conventional-changelog-conventionalcommits extra plugin :bug: ([b6d100b](https://github.com/LerianStudio/midaz-private/commit/b6d100b928d18d2a35331a87feca50c779c8447f))
* command sql ([5cf410f](https://github.com/LerianStudio/midaz-private/commit/5cf410fc6b7eef63698ff4cbc3c48eea7651b3e4))
* final adjustments ([9ad840e](https://github.com/LerianStudio/midaz-private/commit/9ad840ef0e1e39cad31288cc9b39bbd368d575e0))
* gofmt ([a9f0544](https://github.com/LerianStudio/midaz-private/commit/a9f0544a38508e9d3b37794a1b44af88216c63bb))
* handlers and routes ([98ba8ea](https://github.com/LerianStudio/midaz-private/commit/98ba8eae8369e85727292ba7ecc66c792f9390d3))
* interface and postgres implementation ([ae4fa6f](https://github.com/LerianStudio/midaz-private/commit/ae4fa6ffdba9f5f91c169611eea864e3efb3cb09))
* lint ([23bdd49](https://github.com/LerianStudio/midaz-private/commit/23bdd49daced9c6040134706871a6c7811d267fe))
* make sec and make lint ([fac8e3a](https://github.com/LerianStudio/midaz-private/commit/fac8e3a392a5139236fd8dab1badb656e7e2fc35))
* migrations ([82c82ba](https://github.com/LerianStudio/midaz-private/commit/82c82ba7b20c966d5dc6de13937c3386b21cc699))
* query sql ([fdc2de8](https://github.com/LerianStudio/midaz-private/commit/fdc2de8841246382ed4cf7edf6026990e041f187))
* **divisions:** remove everything from divisions ([5cbed6e](https://github.com/LerianStudio/midaz-private/commit/5cbed6e67ad219ef7aacb4190121f1b6ce804999))
* remove immudb from ledger ([6264110](https://github.com/LerianStudio/midaz-private/commit/6264110af51d4d9d2222d760be91a2983ee4f050))
* template ([5519aa2](https://github.com/LerianStudio/midaz-private/commit/5519aa2d614bb845108b87f967b91c32814040f8))
* tests ([4c3be58](https://github.com/LerianStudio/midaz-private/commit/4c3be58a69a79e2aec1453a93dc7b411388ddac4))

## [1.11.0-beta.1](https://github.com/LerianStudio/midaz-private/compare/v1.10.0...v1.11.0-beta.1) (2024-04-30)

## [1.10.0](https://github.com/LerianStudio/midaz-private/compare/v1.9.0...v1.10.0) (2024-04-25)


### Features

* **doc:** add first version of open api doc ([16b3bc7](https://github.com/LerianStudio/midaz-private/commit/16b3bc7d462a7e9ee2b81e1db7976d0322a9a202))
* **doc:** add initial swagger impl ([d50a18b](https://github.com/LerianStudio/midaz-private/commit/d50a18b368416f35eb0028010fc1cfd241654d4d))
* Add primary and replica immudb to the transaction domain, along with improvements such as variable renaming. ([b68d76a](https://github.com/LerianStudio/midaz-private/commit/b68d76a042f3844ead90c46adcca4eca4cbaca3c))
* **doc:** introduce updated version of doc ([048fee7](https://github.com/LerianStudio/midaz-private/commit/048fee79d2c1f427689f37c50b41202b3666c6ab))


### Bug Fixes

* **metadata:** add length validation in metadata fields key and value ([d7faaad](https://github.com/LerianStudio/midaz-private/commit/d7faaad7cac780d99014cf95cc8725d5e7a8caa3))
* **doc:** adjust doc path ([244aae7](https://github.com/LerianStudio/midaz-private/commit/244aae7f0334c9a781f2bc47673a3dd90f4a28af))
* **lint:** adjust linter issues ([9dd364f](https://github.com/LerianStudio/midaz-private/commit/9dd364fa8b5af52ca290feb8c135650c94d2f21f))
* **linter:** adjust linter issues ([9ebc80b](https://github.com/LerianStudio/midaz-private/commit/9ebc80b55a246e044054ce78bb43e9c2cbca5d9e))
* error merge ([8da0131](https://github.com/LerianStudio/midaz-private/commit/8da013131f9a57ae5fdd2011d02c16493a230d4d))
* **metadata:** remove empty-lines extra empty line at the start of a block ([5837adf](https://github.com/LerianStudio/midaz-private/commit/5837adf877bccd641cf22f64c34f02c790500271))
* removing fake secrets from .env.example :bug: ([700fc11](https://github.com/LerianStudio/midaz-private/commit/700fc110e78fa14203df39b812a55bfcbf7d5f01))
* removing fake secrets from .env.example :bug: ([8c025f0](https://github.com/LerianStudio/midaz-private/commit/8c025f05c8cd1378dbf377e9d730dd8206d5f871))
* removing one immudb common :bug: ([35f1e43](https://github.com/LerianStudio/midaz-private/commit/35f1e4366a362fddc4c605937cd2eb27c4fffc06))
* removing one immudb common :bug: ([e0b7aae](https://github.com/LerianStudio/midaz-private/commit/e0b7aae1bf0d05fb7117f5eb7ee737b3b3f4c4bf))

## [1.10.0-beta.3](https://github.com/LerianStudio/midaz-private/compare/v1.10.0-beta.2...v1.10.0-beta.3) (2024-04-25)


### Features

* **doc:** add first version of open api doc ([16b3bc7](https://github.com/LerianStudio/midaz-private/commit/16b3bc7d462a7e9ee2b81e1db7976d0322a9a202))
* **doc:** add initial swagger impl ([d50a18b](https://github.com/LerianStudio/midaz-private/commit/d50a18b368416f35eb0028010fc1cfd241654d4d))
* Add primary and replica immudb to the transaction domain, along with improvements such as variable renaming. ([b68d76a](https://github.com/LerianStudio/midaz-private/commit/b68d76a042f3844ead90c46adcca4eca4cbaca3c))
* **doc:** introduce updated version of doc ([048fee7](https://github.com/LerianStudio/midaz-private/commit/048fee79d2c1f427689f37c50b41202b3666c6ab))


### Bug Fixes

* **metadata:** add length validation in metadata fields key and value ([d7faaad](https://github.com/LerianStudio/midaz-private/commit/d7faaad7cac780d99014cf95cc8725d5e7a8caa3))
* **doc:** adjust doc path ([244aae7](https://github.com/LerianStudio/midaz-private/commit/244aae7f0334c9a781f2bc47673a3dd90f4a28af))
* **linter:** adjust linter issues ([9ebc80b](https://github.com/LerianStudio/midaz-private/commit/9ebc80b55a246e044054ce78bb43e9c2cbca5d9e))
* error merge ([8da0131](https://github.com/LerianStudio/midaz-private/commit/8da013131f9a57ae5fdd2011d02c16493a230d4d))
* **metadata:** remove empty-lines extra empty line at the start of a block ([5837adf](https://github.com/LerianStudio/midaz-private/commit/5837adf877bccd641cf22f64c34f02c790500271))
* removing fake secrets from .env.example :bug: ([700fc11](https://github.com/LerianStudio/midaz-private/commit/700fc110e78fa14203df39b812a55bfcbf7d5f01))
* removing fake secrets from .env.example :bug: ([8c025f0](https://github.com/LerianStudio/midaz-private/commit/8c025f05c8cd1378dbf377e9d730dd8206d5f871))
* removing one immudb common :bug: ([35f1e43](https://github.com/LerianStudio/midaz-private/commit/35f1e4366a362fddc4c605937cd2eb27c4fffc06))
* removing one immudb common :bug: ([e0b7aae](https://github.com/LerianStudio/midaz-private/commit/e0b7aae1bf0d05fb7117f5eb7ee737b3b3f4c4bf))

## [1.10.0-beta.2](https://github.com/LerianStudio/midaz-private/compare/v1.10.0-beta.1...v1.10.0-beta.2) (2024-04-23)

## [1.10.0-beta.1](https://github.com/LerianStudio/midaz-private/compare/v1.9.0...v1.10.0-beta.1) (2024-04-22)


### Bug Fixes

* **lint:** adjust linter issues ([9dd364f](https://github.com/LerianStudio/midaz-private/commit/9dd364fa8b5af52ca290feb8c135650c94d2f21f))

## [1.9.0](https://github.com/LerianStudio/midaz-private/compare/v1.8.0...v1.9.0) (2024-04-19)


### Features

* add func to convert from camel to snake case ([4d49b7e](https://github.com/LerianStudio/midaz-private/commit/4d49b7e1d9575495b89e26106b55cb387cba7f89))
* **MZ-136:** add sort by created_at desc for list queries ([5af3e81](https://github.com/LerianStudio/midaz-private/commit/5af3e8194bafc2f3badfa20d8b5f4effb28b45e6))
* add steps to goreleaser into release workflow :sparkles: ([394470d](https://github.com/LerianStudio/midaz-private/commit/394470d411ea74fbc755a2e938f3a441a5912961))
* **routes:** uncomment portfolio routes ([d16cddc](https://github.com/LerianStudio/midaz-private/commit/d16cddc6ce9972b45435d2bef64886ff163420d0))


### Bug Fixes

* **portfolio:** add missing updated_at logic for update portfolio flow ([b1e572d](https://github.com/LerianStudio/midaz-private/commit/b1e572ddf981fd5dbb236d7cef8503437f2a6308))
* **linter:** adjust linter issues ([cac1b7d](https://github.com/LerianStudio/midaz-private/commit/cac1b7d2f2eb1b24d1e7a56735fae55545e92ef6))
* **linter:** adjust linter issues ([9953697](https://github.com/LerianStudio/midaz-private/commit/99536973603b80e1741d744c130b211107757ce0))
* debug goreleaser ([ab68e55](https://github.com/LerianStudio/midaz-private/commit/ab68e55e3cacb4545b9bc1d37161b68bfc5b4a1e))
* **linter:** remove cuddled declarations ([5a0554c](https://github.com/LerianStudio/midaz-private/commit/5a0554c0340ccc7038ebea98228e07afff25c0e1))
* **linter:** remove usage of interface ([6523326](https://github.com/LerianStudio/midaz-private/commit/6523326bd19238ab786facdc452589770fd448fe))
* **sql:** remove wrong usage of any instead of in for list queries ([b187140](https://github.com/LerianStudio/midaz-private/commit/b1871405df0f762a62ec9a89375bf6408ba104f9))

## [1.9.0-beta.6](https://github.com/LerianStudio/midaz-private/compare/v1.9.0-beta.5...v1.9.0-beta.6) (2024-04-19)


### Bug Fixes

* **portfolio:** add missing updated_at logic for update portfolio flow ([b1e572d](https://github.com/LerianStudio/midaz-private/commit/b1e572ddf981fd5dbb236d7cef8503437f2a6308))

## [1.9.0-beta.5](https://github.com/LerianStudio/midaz-private/compare/v1.9.0-beta.4...v1.9.0-beta.5) (2024-04-19)


### Features

* add func to convert from camel to snake case ([4d49b7e](https://github.com/LerianStudio/midaz-private/commit/4d49b7e1d9575495b89e26106b55cb387cba7f89))
* **MZ-136:** add sort by created_at desc for list queries ([5af3e81](https://github.com/LerianStudio/midaz-private/commit/5af3e8194bafc2f3badfa20d8b5f4effb28b45e6))
* **routes:** uncomment portfolio routes ([d16cddc](https://github.com/LerianStudio/midaz-private/commit/d16cddc6ce9972b45435d2bef64886ff163420d0))


### Bug Fixes

* **linter:** adjust linter issues ([cac1b7d](https://github.com/LerianStudio/midaz-private/commit/cac1b7d2f2eb1b24d1e7a56735fae55545e92ef6))
* **linter:** adjust linter issues ([9953697](https://github.com/LerianStudio/midaz-private/commit/99536973603b80e1741d744c130b211107757ce0))
* debug goreleaser ([ab68e55](https://github.com/LerianStudio/midaz-private/commit/ab68e55e3cacb4545b9bc1d37161b68bfc5b4a1e))
* **linter:** remove cuddled declarations ([5a0554c](https://github.com/LerianStudio/midaz-private/commit/5a0554c0340ccc7038ebea98228e07afff25c0e1))
* **linter:** remove usage of interface ([6523326](https://github.com/LerianStudio/midaz-private/commit/6523326bd19238ab786facdc452589770fd448fe))
* **sql:** remove wrong usage of any instead of in for list queries ([b187140](https://github.com/LerianStudio/midaz-private/commit/b1871405df0f762a62ec9a89375bf6408ba104f9))

## [1.9.0-beta.4](https://github.com/LerianStudio/midaz-private/compare/v1.9.0-beta.3...v1.9.0-beta.4) (2024-04-18)

## [1.9.0-beta.3](https://github.com/LerianStudio/midaz-private/compare/v1.9.0-beta.2...v1.9.0-beta.3) (2024-04-18)

## [1.9.0-beta.2](https://github.com/LerianStudio/midaz-private/compare/v1.9.0-beta.1...v1.9.0-beta.2) (2024-04-18)


### Features

* add steps to goreleaser into release workflow :sparkles: ([394470d](https://github.com/LerianStudio/midaz-private/commit/394470d411ea74fbc755a2e938f3a441a5912961))

## [1.9.0-beta.1](https://github.com/LerianStudio/midaz-private/compare/v1.8.0...v1.9.0-beta.1) (2024-04-18)

## [1.8.0](https://github.com/LerianStudio/midaz-private/compare/v1.7.0...v1.8.0) (2024-04-17)


### Features

* Creating a very cool feature :sparkles: ([b84daf1](https://github.com/LerianStudio/midaz-private/commit/b84daf135b224dd229a22a56d228123e1ede5bf5))

## [1.8.0-beta.1](https://github.com/LerianStudio/midaz-private/compare/v1.7.0...v1.8.0-beta.1) (2024-04-17)


### Features

* Creating a very cool feature :sparkles: ([b84daf1](https://github.com/LerianStudio/midaz-private/commit/b84daf135b224dd229a22a56d228123e1ede5bf5))

## [1.7.0](https://github.com/LerianStudio/midaz-private/compare/v1.6.0...v1.7.0) (2024-04-17)

## [1.7.0-beta.1](https://github.com/LerianStudio/midaz-private/compare/v1.6.0...v1.7.0-beta.1) (2024-04-17)

## [1.6.0](https://github.com/LerianStudio/midaz/compare/v1.5.0...v1.6.0) (2024-04-16)

## [1.6.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.6.0-beta.1...v1.6.0-beta.2) (2024-04-16)

## [1.6.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.5.0...v1.6.0-beta.1) (2024-04-16)

## [1.5.0](https://github.com/LerianStudio/midaz/compare/v1.4.0...v1.5.0) (2024-04-16)


### Features

* Remove slack notifications in release and build jobs :sparkles: ([3c629cb](https://github.com/LerianStudio/midaz/commit/3c629cb7ea635fdc4d7101737f8f2026c418f2b1))

## [1.5.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.4.0...v1.5.0-beta.1) (2024-04-16)


### Features

* Remove slack notifications in release and build jobs :sparkles: ([3c629cb](https://github.com/LerianStudio/midaz/commit/3c629cb7ea635fdc4d7101737f8f2026c418f2b1))

## [1.4.0](https://github.com/LerianStudio/midaz/compare/v1.3.0...v1.4.0) (2024-04-16)


### Bug Fixes

* Remove slack notifications and add changelog notification to Discord :bug: ([315dbd6](https://github.com/LerianStudio/midaz/commit/315dbd616afac5e0c7410d5f65831681b3bb93fe))

## [1.3.0](https://github.com/LerianStudio/midaz/compare/v1.2.0...v1.3.0) (2024-04-16)


### Features

* **portfolio:** refactor portfolio model and migration ([f9f0157](https://github.com/LerianStudio/midaz/commit/f9f015795510e2b1c84e41c5e1678f836bd3de7d))
* remove ignored files in pipelines ([a6f0ace](https://github.com/LerianStudio/midaz/commit/a6f0ace582c7e7c70b4d089abcceab279758db92))


### Bug Fixes

* **envs:** add usage of env vars for replica database ([e243e45](https://github.com/LerianStudio/midaz/commit/e243e4506b10babe0b52efbbf74056c8a0300362))
* **linter:** adjust formatting; adjust line separators ([e9df066](https://github.com/LerianStudio/midaz/commit/e9df066dda7cec40dc520720b64b8e5d63b48c86))
* **compose:** adjust replica database configuration for correct port settings; use of own healthcheck; adjust dependency with primary healthy status ([244f693](https://github.com/LerianStudio/midaz/commit/244f693e625fb13694d9296841ea1cf6e34128a6))
* ajustando o tamanho do map ([67d5177](https://github.com/LerianStudio/midaz/commit/67d5177fd9b8f357f3dec37930d5b40fcfba4cea))
* ajuste na classe get-all-accounts ([ded8579](https://github.com/LerianStudio/midaz/commit/ded8579784966369be59d2cf54e0a8c67e58b12f))
* lint ajustes ([a09e718](https://github.com/LerianStudio/midaz/commit/a09e718efd511679106a4e9ac3fffd1c4ff7caa6))
* Rollback line :bug: ([da4c101](https://github.com/LerianStudio/midaz/commit/da4c1012a84bb5d8eedd578aed16896d28884d06))
* **sec:** update dependencies version to patch vulnerabilities ([40dc35f](https://github.com/LerianStudio/midaz/commit/40dc35faf244cf24642d830e91fea41237068ead))

## [1.3.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.2.0...v1.3.0-beta.1) (2024-04-16)


### Features

* **portfolio:** refactor portfolio model and migration ([f9f0157](https://github.com/LerianStudio/midaz/commit/f9f015795510e2b1c84e41c5e1678f836bd3de7d))
* remove ignored files in pipelines ([a6f0ace](https://github.com/LerianStudio/midaz/commit/a6f0ace582c7e7c70b4d089abcceab279758db92))


### Bug Fixes

* **envs:** add usage of env vars for replica database ([e243e45](https://github.com/LerianStudio/midaz/commit/e243e4506b10babe0b52efbbf74056c8a0300362))
* **linter:** adjust formatting; adjust line separators ([e9df066](https://github.com/LerianStudio/midaz/commit/e9df066dda7cec40dc520720b64b8e5d63b48c86))
* **compose:** adjust replica database configuration for correct port settings; use of own healthcheck; adjust dependency with primary healthy status ([244f693](https://github.com/LerianStudio/midaz/commit/244f693e625fb13694d9296841ea1cf6e34128a6))
* ajustando o tamanho do map ([67d5177](https://github.com/LerianStudio/midaz/commit/67d5177fd9b8f357f3dec37930d5b40fcfba4cea))
* ajuste na classe get-all-accounts ([ded8579](https://github.com/LerianStudio/midaz/commit/ded8579784966369be59d2cf54e0a8c67e58b12f))
* lint ajustes ([a09e718](https://github.com/LerianStudio/midaz/commit/a09e718efd511679106a4e9ac3fffd1c4ff7caa6))
* Rollback line :bug: ([da4c101](https://github.com/LerianStudio/midaz/commit/da4c1012a84bb5d8eedd578aed16896d28884d06))
* **sec:** update dependencies version to patch vulnerabilities ([40dc35f](https://github.com/LerianStudio/midaz/commit/40dc35faf244cf24642d830e91fea41237068ead))

## [1.2.0](https://github.com/LerianStudio/midaz/compare/v1.1.0...v1.2.0) (2024-04-15)


### Features

* split test jobs + add CODEOWNERS file + dependabot config :sparkles: ([04d1a57](https://github.com/LerianStudio/midaz/commit/04d1a57f15692cd1bf54b7ba37b1832165bcbeb5))


### Bug Fixes

* codeowners rules :bug: ([45e3abb](https://github.com/LerianStudio/midaz/commit/45e3abbd70dd4516c0e063ba57dda4d7615976d1))

## [1.2.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.1.0...v1.2.0-beta.1) (2024-04-15)


### Features

* split test jobs + add CODEOWNERS file + dependabot config :sparkles: ([04d1a57](https://github.com/LerianStudio/midaz/commit/04d1a57f15692cd1bf54b7ba37b1832165bcbeb5))


### Bug Fixes

* codeowners rules :bug: ([45e3abb](https://github.com/LerianStudio/midaz/commit/45e3abbd70dd4516c0e063ba57dda4d7615976d1))

## [1.1.0](https://github.com/LerianStudio/midaz/compare/v1.0.3...v1.1.0) (2024-04-14)


### Features

* **mpostgres:** Add create, update, delete functions :sparkles: ([bb993c7](https://github.com/LerianStudio/midaz/commit/bb993c784f192898b65e65b4af3c4ec20f40afa0))
* **database:** Add dbresolver for primary and replica DBs :sparkles: ([de73be2](https://github.com/LerianStudio/midaz/commit/de73be261dcc8a0ee67f849918f0689ebc81afc6))
* **common:** Add generic Contains function to utils :sparkles: ([0122d60](https://github.com/LerianStudio/midaz/commit/0122d60aaaf284bbd0975f06bcd61e20fa4f4a0e))
* add gpg sign to bot commits :sparkles: ([a0169e4](https://github.com/LerianStudio/midaz/commit/a0169e46d7399078c7dd2bc183a2616fe3b31d49))
* add gpg sign to bot commits :sparkles: ([62c95f0](https://github.com/LerianStudio/midaz/commit/62c95f0e11b22c8cd4414c58c134dfa41624b11d))
* **common:** Add pointer and string utilities, update account fields :sparkles: ([e783f4a](https://github.com/LerianStudio/midaz/commit/e783f4a2cc83ebb8d729351bc7ddd29b3813c6f2))
* **mpostgres:** Add SQL query builder and repository methods :sparkles: ([23294a2](https://github.com/LerianStudio/midaz/commit/23294a2760464b3f2811cd56be06a6a2b64a2d3a))
* **database connection:** Enable MongoDB connection and fix docker-compose :sparkles: ([990c5f0](https://github.com/LerianStudio/midaz/commit/990c5f09dadd07c8480b99665fd4f41137f4d4b3))


### Bug Fixes

* debug gpg sign :bug: ([a0d7c78](https://github.com/LerianStudio/midaz/commit/a0d7c78b4a656a9d5fbf158b3d65500d58b7fa7a))
* **ledger:** update host in pg_basebackup command :bug: ([1bb3d38](https://github.com/LerianStudio/midaz/commit/1bb3d38c708aa135e58960932153d0ff3d3ad636))

## [1.1.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.1.0-beta.3...v1.1.0-beta.4) (2024-04-14)


### Features

* **mpostgres:** Add create, update, delete functions :sparkles: ([bb993c7](https://github.com/LerianStudio/midaz/commit/bb993c784f192898b65e65b4af3c4ec20f40afa0))
* **database:** Add dbresolver for primary and replica DBs :sparkles: ([de73be2](https://github.com/LerianStudio/midaz/commit/de73be261dcc8a0ee67f849918f0689ebc81afc6))
* **common:** Add generic Contains function to utils :sparkles: ([0122d60](https://github.com/LerianStudio/midaz/commit/0122d60aaaf284bbd0975f06bcd61e20fa4f4a0e))
* **common:** Add pointer and string utilities, update account fields :sparkles: ([e783f4a](https://github.com/LerianStudio/midaz/commit/e783f4a2cc83ebb8d729351bc7ddd29b3813c6f2))
* **mpostgres:** Add SQL query builder and repository methods :sparkles: ([23294a2](https://github.com/LerianStudio/midaz/commit/23294a2760464b3f2811cd56be06a6a2b64a2d3a))
* **database connection:** Enable MongoDB connection and fix docker-compose :sparkles: ([990c5f0](https://github.com/LerianStudio/midaz/commit/990c5f09dadd07c8480b99665fd4f41137f4d4b3))


### Bug Fixes

* **ledger:** update host in pg_basebackup command :bug: ([1bb3d38](https://github.com/LerianStudio/midaz/commit/1bb3d38c708aa135e58960932153d0ff3d3ad636))

## [1.1.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.1.0-beta.2...v1.1.0-beta.3) (2024-04-12)

## [1.1.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.1.0-beta.1...v1.1.0-beta.2) (2024-04-12)

## [1.1.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.0.4-beta.2...v1.1.0-beta.1) (2024-04-12)


### Features

* add gpg sign to bot commits :sparkles: ([a0169e4](https://github.com/LerianStudio/midaz/commit/a0169e46d7399078c7dd2bc183a2616fe3b31d49))
* add gpg sign to bot commits :sparkles: ([62c95f0](https://github.com/LerianStudio/midaz/commit/62c95f0e11b22c8cd4414c58c134dfa41624b11d))


### Bug Fixes

* debug gpg sign :bug: ([a0d7c78](https://github.com/LerianStudio/midaz/commit/a0d7c78b4a656a9d5fbf158b3d65500d58b7fa7a))

## [1.1.0-beta.1](https://github.com/LerianStudio/midaz/compare/v1.0.4-beta.2...v1.1.0-beta.1) (2024-04-12)


### Features

* add gpg sign to bot commits :sparkles: ([a0169e4](https://github.com/LerianStudio/midaz/commit/a0169e46d7399078c7dd2bc183a2616fe3b31d49))
* add gpg sign to bot commits :sparkles: ([62c95f0](https://github.com/LerianStudio/midaz/commit/62c95f0e11b22c8cd4414c58c134dfa41624b11d))

## [1.0.4-beta.2](https://github.com/LerianStudio/midaz/compare/v1.0.4-beta.1...v1.0.4-beta.2) (2024-04-12)

## [1.0.4-beta.1](https://github.com/LerianStudio/midaz/compare/v1.0.3...v1.0.4-beta.1) (2024-04-11)

## [1.0.3](https://github.com/LerianStudio/midaz/compare/v1.0.2...v1.0.3) (2024-04-11)

## [1.0.3-beta.1](https://github.com/LerianStudio/midaz/compare/v1.0.2...v1.0.3-beta.1) (2024-04-11)

## [1.0.2](https://github.com/LerianStudio/midaz/compare/v1.0.1...v1.0.2) (2024-04-11)

## [1.0.2-beta.1](https://github.com/LerianStudio/midaz/compare/v1.0.1...v1.0.2-beta.1) (2024-04-11)

## [1.0.1](https://github.com/LerianStudio/midaz/compare/v1.0.0...v1.0.1) (2024-04-11)

## [1.0.1-beta.6](https://github.com/LerianStudio/midaz/compare/v1.0.1-beta.5...v1.0.1-beta.6) (2024-04-11)

## [1.0.1-beta.5](https://github.com/LerianStudio/midaz/compare/v1.0.1-beta.4...v1.0.1-beta.5) (2024-04-11)

## [1.0.1-beta.4](https://github.com/LerianStudio/midaz/compare/v1.0.1-beta.3...v1.0.1-beta.4) (2024-04-11)

## [1.0.1-beta.3](https://github.com/LerianStudio/midaz/compare/v1.0.1-beta.2...v1.0.1-beta.3) (2024-04-11)

## [1.0.1-beta.2](https://github.com/LerianStudio/midaz/compare/v1.0.1-beta.1...v1.0.1-beta.2) (2024-04-11)

## [1.0.1-beta.1](https://github.com/LerianStudio/midaz/compare/v1.0.0...v1.0.1-beta.1) (2024-04-11)

## [1.0.0-beta.8](https://github.com/LerianStudio/midaz/compare/v1.0.0-beta.7...v1.0.0-beta.8) (2024-04-11)


### Bug Fixes

* app name to dockerhub push ([7d1400d](https://github.com/LerianStudio/midaz/commit/7d1400db642dce8df87a4b931969fc9c5177024e))

## [1.0.0-beta.7](https://github.com/LerianStudio/midaz/compare/v1.0.0-beta.6...v1.0.0-beta.7) (2024-04-11)


### Bug Fixes

* fix comma ([0db9660](https://github.com/LerianStudio/midaz/commit/0db9660729203937529885effa5c5996f5c75f67))

## [1.0.0-beta.7](https://github.com/LerianStudio/midaz/compare/v1.0.0-beta.6...v1.0.0-beta.7) (2024-04-11)

## [1.0.0-beta.6](https://github.com/LerianStudio/midaz/compare/v1.0.0-beta.5...v1.0.0-beta.6) (2024-04-11)

## [1.0.0-beta.5](https://github.com/LerianStudio/midaz/compare/v1.0.0-beta.4...v1.0.0-beta.5) (2024-04-11)

## [1.0.0-beta.4](https://github.com/LerianStudio/midaz/compare/v1.0.0-beta.3...v1.0.0-beta.4) (2024-04-11)

## [1.0.0-beta.3](https://github.com/LerianStudio/midaz/compare/v1.0.0-beta.2...v1.0.0-beta.3) (2024-04-11)

## [1.0.0-beta.2](https://github.com/LerianStudio/midaz/compare/v1.0.0-beta.1...v1.0.0-beta.2) (2024-04-11)


### Bug Fixes

* identation ([5796b66](https://github.com/LerianStudio/midaz/commit/5796b662b737fa4a26c7bb9cc575d95fbb91b357))

## 1.0.0-beta.1 (2024-04-11)


### Features

* add accounts testes ([b621dc1](https://github.com/LerianStudio/midaz/commit/b621dc142a8a04514d7477b89abe72f03be3beaa))
* **shell:** Add ASCII and color shell scripts ([d079910](https://github.com/LerianStudio/midaz/commit/d079910b467e8b6429cbf351222482afebc7a250))
* add child-account testes ([e0620eb](https://github.com/LerianStudio/midaz/commit/e0620eb5de05ef498a53c2b1a659f0e93444ef28))
* **Makefile:** Add cover test command :sparkles: ([f549db3](https://github.com/LerianStudio/midaz/commit/f549db3d18f55be1273b76640c604aea6a448ff7))
* **NoSQL:** Add Create metadata with id organization ([d70b5d7](https://github.com/LerianStudio/midaz/commit/d70b5d7364ae025deb0f676c27e867a7dda9c046))
* add DDL scripts for database migration ([25e5df3](https://github.com/LerianStudio/midaz/commit/25e5df35ea5585dbff69df330a4d3af70b0ed93b))
* **Organization:** Add Delete ([ee76903](https://github.com/LerianStudio/midaz/commit/ee76903780547ea612ce0e2490c1878714d074e2))
* **NoSQL:** Add dpdate & delete metadata on mongodb ([44bf06e](https://github.com/LerianStudio/midaz/commit/44bf06ea9cd394d2d57cc89dff52dabec7168c59))
* **mpostgres:** Add file system migration source :sparkles: ([a776433](https://github.com/LerianStudio/midaz/commit/a7764332079bf00ee8cc504e8978b50181d4d0ec))
* **organization:** Add find functionality for organization ([96049ef](https://github.com/LerianStudio/midaz/commit/96049ef44841458252f618fff1a93e49d9c88984))
* add generate and create mocks :sparkles: ([1d8ffa0](https://github.com/LerianStudio/midaz/commit/1d8ffa08bf4535eff60deba3204b6d2ffb88039d))
* **NoSQL:** Add Get all Organizations and add your own Metadata ([33804e0](https://github.com/LerianStudio/midaz/commit/33804e020b5c3e6f31b20d023c0867c0619cfb40))
* **NoSQL:** Add Get all Organizations by Metadata ([4acb1fb](https://github.com/LerianStudio/midaz/commit/4acb1fb875656b3b0a7578903772d4d9198db36d))
* **Organization:** Add Get All ([2bf231a](https://github.com/LerianStudio/midaz/commit/2bf231ae4a6e623ec39fc857e47c8a28b1be3878))
* **NoSQL:** Add Get metadata with id organization ([afb4bbd](https://github.com/LerianStudio/midaz/commit/afb4bbdc3059a4c1bfe88f8ccd677f8bfe08ba47))
* **auth:** add initial auth configuration for ory stack usage ([1c0c621](https://github.com/LerianStudio/midaz/commit/1c0c621a7b0e29992e1cb0183674ae69ecc9e52c))
* add instrument testes ([c4a9cc0](https://github.com/LerianStudio/midaz/commit/c4a9cc0773a8920e569b92182b6bf758d7787083))
* **NoSQL:** Add libs mongodb ([28fbfaf](https://github.com/LerianStudio/midaz/commit/28fbfafcad304436afb32d986e477468bf38c4f3))
* **create-division:** Add metadata creation to CreateDivision ([67fc945](https://github.com/LerianStudio/midaz/commit/67fc945a575d1452c3f183e2bef50f49f7999ba0))
* add metadata testes ([e2cc055](https://github.com/LerianStudio/midaz/commit/e2cc05569bdd40285e9349f0cd41d5dbbc37673e))
* **NoSQL:** Add mongodb on docker-compose ([88b81ab](https://github.com/LerianStudio/midaz/commit/88b81abc8654a30789e3b191dbf48ffa2a7f30eb))
* **ledger:** Add new ledger API components ([c657d3d](https://github.com/LerianStudio/midaz/commit/c657d3da7bb2ac8d66a8a47c8d4422519026df5e))
* add portfolio testes ([78e2727](https://github.com/LerianStudio/midaz/commit/78e2727e3fc9cf24100ec9f0f1d8881f33483a61))
* **components:** Add security scan and improve http client :sparkles: ([78d9736](https://github.com/LerianStudio/midaz/commit/78d973655e8e739b8a8f5c7eeb04f285f428e587))
* **postgres:** add source database name to connection struct ([39b22d2](https://github.com/LerianStudio/midaz/commit/39b22d2b62c66a105af0e7c7820f293324987122))
* **organization:** Add status field to Organization model ([72283b3](https://github.com/LerianStudio/midaz/commit/72283b3298a2aab2ab506889d7d2fbab6a9d0aa4))
* **Organization:** Add Update ([0b01ac0](https://github.com/LerianStudio/midaz/commit/0b01ac0bd597d44db27678ff33e248f4de6d76eb))
* **Product:** Add ([6789c74](https://github.com/LerianStudio/midaz/commit/6789c7452b4171aaa3a1ae468beeb30136adc1e4))
* **NoSQL:** Adjusts and add redis on docker-compose.yaml only ([5122bf3](https://github.com/LerianStudio/midaz/commit/5122bf31d9f41eef14c822445ba35c135c3a6b26))
* **NoSQL:** Config geral ([2a7f3ef](https://github.com/LerianStudio/midaz/commit/2a7f3ef29f43ba7eabc282a281ca797a734b9270))
* **Divisions:** Create divisions and some adjusts ([8bfe439](https://github.com/LerianStudio/midaz/commit/8bfe439d7f0bbec6918b4ab015376531a44a8d9c))
* **Ledger:** Create Ledger ([afae31b](https://github.com/LerianStudio/midaz/commit/afae31bd42c58158bbe1f45d468a1550489b0ee7))
* **Account:** CREATE ([2d91261](https://github.com/LerianStudio/midaz/commit/2d912611dcfc303574fd6e30c67ddc1d875a77ec))
* **chiuld-account:** create ([4ebfed1](https://github.com/LerianStudio/midaz/commit/4ebfed11415771412fb57e51c368b7a7c41c5c30))
* **Portfolio:** Create ([87b6840](https://github.com/LerianStudio/midaz/commit/87b684088a13648aaddb2f5b8f4084ec9c0daf4f))
* **instrument:** crud ([2b29335](https://github.com/LerianStudio/midaz/commit/2b2933531406810517d8ee95d7cc17e573b326de))
* **Division:** Delete Division ([50d87e7](https://github.com/LerianStudio/midaz/commit/50d87e7be5c621f28c418ea986fa6182a2013c89))
* **Ledger:** Delete Ledger ([b1900e4](https://github.com/LerianStudio/midaz/commit/b1900e4e3147ae68aecd79fcf964b8bc139d4350))
* **account:** delete ([bf766c7](https://github.com/LerianStudio/midaz/commit/bf766c744254416280dcf7597c0540ca44cb9bb5))
* **child-account:** delete ([ddbfdf9](https://github.com/LerianStudio/midaz/commit/ddbfdf9f99eb36454b761d7dbc8f0d49e064c9ba))
* **Portfolio:** Delete ([86ab6c4](https://github.com/LerianStudio/midaz/commit/86ab6c439fe4b24d9ee3dc1df2b0b76306eab76f))
* **Product:** Delete ([6bd1519](https://github.com/LerianStudio/midaz/commit/6bd1519c994eb09560b38c538d03fc9e5a4cae07))
* division add tests :sparkles: ([8708d36](https://github.com/LerianStudio/midaz/commit/8708d364581590ad193f24f959b48f961a750679))
* **ledger:** Enable ledger repository and handler ([1139a92](https://github.com/LerianStudio/midaz/commit/1139a92001fe2ff908299b1e5d6649851216ee45))
* **ledger:** Enable ledger use case operations ([ff70c70](https://github.com/LerianStudio/midaz/commit/ff70c70ff39604a46c67b0bd75e3d45ac2cc87a2))
* **Division:** Get all divisions and get all divisions by Metadata ([4888367](https://github.com/LerianStudio/midaz/commit/48883671cd81e1b3d4d1ed37a8a149ea0935cd95))
* **Ledger:** Get all Ledgers and get all Ledgers by Metadata ([ec4db79](https://github.com/LerianStudio/midaz/commit/ec4db79ccd003dffa2b0b9cf7cda645c477655be))
* **chiuld-account:** get all ([3092638](https://github.com/LerianStudio/midaz/commit/30926384a9e8a07799a5c879f249c49c1d3e46f7))
* **Portfolio:** Get All ([2ed3ed1](https://github.com/LerianStudio/midaz/commit/2ed3ed14616479d1272bc5683946fe67dfd34d5b))
* **Product:** Get All ([66503ab](https://github.com/LerianStudio/midaz/commit/66503ab8569c58712b62e5d5a0a277cbeb1092dd))
* **Product:** Get All ([f928ad5](https://github.com/LerianStudio/midaz/commit/f928ad51b5aefef21f07d9942158a1ce95f1cdf5))
* **Account:** GET BY ID ([4dd8ba6](https://github.com/LerianStudio/midaz/commit/4dd8ba61bd5722ca3a3b00345e74b0bebe7623e0))
* **child-account:** get by id ([d217ded](https://github.com/LerianStudio/midaz/commit/d217ded8e8bc3c2be20c020f9543d72b1b96f032))
* **chiuld-account:** get by id ([2933571](https://github.com/LerianStudio/midaz/commit/29335717a9bab4226b7060fb596e462939783ab6))
* **Portfolio:** Get By ID ([25e6e27](https://github.com/LerianStudio/midaz/commit/25e6e27ab53a007fd7b0ba23ef043d9b58782a90))
* **Product:** Get By Id ([7a382c6](https://github.com/LerianStudio/midaz/commit/7a382c6bcba88bf988d1e169ae385d0974be8fef))
* **Division:** Get division by id organization and id division ([574b226](https://github.com/LerianStudio/midaz/commit/574b2260ed447710ae1e908ebca65cc932debf64))
* **Ledger:** Get Ledger by ID ([c37f64f](https://github.com/LerianStudio/midaz/commit/c37f64fa3fecc36fa020795134a0ced0500adc43))
* **mdz:** go.mod ([dd0bcf9](https://github.com/LerianStudio/midaz/commit/dd0bcf9bb05e5ff84c4466cf232ede9f133b01ca))
* **ledger:** Implement organization model and repo ([6cefe6c](https://github.com/LerianStudio/midaz/commit/6cefe6c30df0e8107af391ac46cd157e59f08227))
* ledger add tests :sparkles: ([17e9b1d](https://github.com/LerianStudio/midaz/commit/17e9b1d4f3da1d1b3591d4583cc7e7eb75900a18))
* **mdz:** login, ui and version commands. auth & ledger boilerplate ([5127802](https://github.com/LerianStudio/midaz/commit/512780223723d0d498d2bf4c13bcac97749927c4))
* **Portfolio:** Metadata and productId ([514e978](https://github.com/LerianStudio/midaz/commit/514e97827f53c2307b327da748425a0b2c802c1b))
* **organization:** organization add tests :sparkles: ([f59154d](https://github.com/LerianStudio/midaz/commit/f59154da934e35d22e04b0ed232ec9677db0316f))
* **instrument:** postman ([959eec7](https://github.com/LerianStudio/midaz/commit/959eec7fa281d90e95658c5d96150b7c7b8eb6a4))
* product add tests :sparkles: ([55b517a](https://github.com/LerianStudio/midaz/commit/55b517aa376b69db8544f60f6df694bc5c37fe40))
* **command:** test create organization ([df5ec02](https://github.com/LerianStudio/midaz/commit/df5ec02d6bd3b4d9dc2c5cf40d27c47302dfecd5))
* **Division:** Update Division ([7e61a02](https://github.com/LerianStudio/midaz/commit/7e61a02e46e0d4a74c451116a9c081a215a467b5))
* **Ledger:** Update Ledger ([c92f8bd](https://github.com/LerianStudio/midaz/commit/c92f8bd08bebdc9e6f4c40a329367d4fd8e3876a))
* **Account:** UPDATE ([0ee68e0](https://github.com/LerianStudio/midaz/commit/0ee68e06a8ad8551689c6d921a633a9d7aa343fd))
* **chiuld-account:** update ([186ad62](https://github.com/LerianStudio/midaz/commit/186ad621f893bc5b31b73cea73caf32bffd16a2f))
* **Portfolio:** Update ([a74e8f1](https://github.com/LerianStudio/midaz/commit/a74e8f13d4f350561860cc9b7fa9a0de4b17790f))
* **Product:** Update ([1da5729](https://github.com/LerianStudio/midaz/commit/1da5729266cb78236ebb624a1e60f0df904b7703))


### Bug Fixes

* add parameter to fetch and change token ([132b6aa](https://github.com/LerianStudio/midaz/commit/132b6aa12eb4666079480fabb330a907853cc9ac))
* **auth:** adjust compose and .env usage based on project goals and standards ([b3536ba](https://github.com/LerianStudio/midaz/commit/b3536ba2c2c893a803039307778b2defcaad829a))
* adjust database configuration ([5bc8558](https://github.com/LerianStudio/midaz/commit/5bc85587de0164a8b8e935d61a34b4720bf50f4f))
* **auth:** adjust directories usage based on project goals and standards ([37b10d4](https://github.com/LerianStudio/midaz/commit/37b10d4ab120f9e8a6669bc3dcf7a9f90f823c8d))
* **division:** adjust method name :bug: ([6c4a154](https://github.com/LerianStudio/midaz/commit/6c4a154067f6cc2595be278312c99eec5c5f5f73))
* change token ([d29805a](https://github.com/LerianStudio/midaz/commit/d29805a66ef563f25afcb989368466302889a925))
* **ledger:** Correct typo in Dockerfile build command :bug: ([7ffecf2](https://github.com/LerianStudio/midaz/commit/7ffecf20188ffab444d498b12f27f588ff23a9b3))
* create test and some lints :bug: ([82ef4b8](https://github.com/LerianStudio/midaz/commit/82ef4b8c4a841a80c4fa84ee483da1887e6c749d))
* debug file :bug: ([54047b0](https://github.com/LerianStudio/midaz/commit/54047b0af88c5ea1995b213141c08b3579695842))
* debug semantic-release ([399322c](https://github.com/LerianStudio/midaz/commit/399322c78dd7c9206c882f643b232d759d27af72))
* disable job and fix syntax :bug: ([958d002](https://github.com/LerianStudio/midaz/commit/958d002053dc246cca79915d16e454ea1be7dcd4))
* fix :bug: ([70d7fa3](https://github.com/LerianStudio/midaz/commit/70d7fa3d85dd922cb1c2f1eca218c6d2cbecae80))
* **ledger:** fix and refactor some adjusts :bug: ([2ca0d63](https://github.com/LerianStudio/midaz/commit/2ca0d63836bb43e442715c74811fa49e0b09b72d))
* fix args :bug: ([e7f73d6](https://github.com/LerianStudio/midaz/commit/e7f73d6896452c33bf9ee885b652b6452a00c4ae))
* fix extra plugins for semantic-release :bug: ([e98b558](https://github.com/LerianStudio/midaz/commit/e98b558c6d451dc43bcdc6c0204e9aae03b44ade))
* fix permission :bug: ([762101a](https://github.com/LerianStudio/midaz/commit/762101a74300d886c25269fcc2a1d709d2c0d662))
* fix script output :bug: ([97a59b4](https://github.com/LerianStudio/midaz/commit/97a59b471547d18e8fa4e1fabb410b12aff9b2bf))
* fix semantic-release behavior :bug: ([1883b39](https://github.com/LerianStudio/midaz/commit/1883b391406e7d50dfcc7720a9ba1ca6d2b0b6a8))
* fix syntax :bug: ([365bbd8](https://github.com/LerianStudio/midaz/commit/365bbd84e94ab0eb2459e2bd4b653d0a7d60dfdd))
* identation ([9d3ec69](https://github.com/LerianStudio/midaz/commit/9d3ec694ae0e7e6646af85b29d15125afbd63769))
* move replication file to folder migration :bug: ([53db96e](https://github.com/LerianStudio/midaz/commit/53db96e6a24794bc7f897d641012cceaf19415ea))
* move replication file to folder setup :bug: ([348a8da](https://github.com/LerianStudio/midaz/commit/348a8da57a70f6cb73145db2bc6afd2363c3d284))
* PR suggestions of @Ralphbaer implemented :bug: ([8cbc696](https://github.com/LerianStudio/midaz/commit/8cbc69628a2b848e632b31ab839d6ddf047064b0))
* remove auto-migration from DB connection process ([7ab1501](https://github.com/LerianStudio/midaz/commit/7ab1501492dc9909cb8eb2454c8b0ec24d413baa))
* remove rule to exclude path :bug: ([48b2cf8](https://github.com/LerianStudio/midaz/commit/48b2cf8607d6b49e8cdd7c4f001448a5dfdaa666))
* remove wrong rule :bug: ([b732416](https://github.com/LerianStudio/midaz/commit/b732416c1e69094c1fa2526159f39e0eaee0cc1f))
* semantic-release ([138e1cc](https://github.com/LerianStudio/midaz/commit/138e1cca68d5b250222001645e608aef8b2c7b77))
* Update merge-back.yml ([5c90141](https://github.com/LerianStudio/midaz/commit/5c901412f9daff57e16013e38a8fbc1ac93222c2))
