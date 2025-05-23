# Project Structure Overview

Welcome to the comprehensive guide on the structure of our project, which is designed with a focus on scalability, maintainability, and clear separation of concerns in line with NextJs UI and API Layer. This architecture not only enhances our project's efficiency and performance but also ensures that our codebase is organized in a way that allows developers to navigate and contribute effectively.

#### Directory Layout

The project is structured into several key directories, each serving specific roles:

```
├── locales
├── public
|   ├── images
|   └── svg
├── scripts
├── services
└── src/
    ├── app/
    │   ├── (auth-routes)
    │   |   └──  signin
    │   ├── (styles/)
    │   |   ├── [...not_found]
    │   |   ├── ledgers
    │   |   |   └── [id]
    │   |   |       ├── accounts-and-portfolios
    │   |   |       ├── overview
    │   |   |       └── segments
    │   |   ├── settings
    │   |   |   └── organizations
    │   └── api
    |       ├── admin
    |       |   ├── alive
    |       |   └── ready
    |       ├── auth
    |       |   └── [...nextauth]
    |       ├── organizations
    |       |   └── [id]
    |       |       └── ledgers
    |       |           ├── [ledgerId]
    |       |           |   └── assets
    |       |           |   |   └── [assetId]
    |       |           |   ├── portfolios
    |       |           |   |   └── [portfolioId]
    |       |           |   └── segments
    |       |           |       └── [segmentId]
    |       |           |
    |       |           └── ledgers-assets
    |       └── utils
    ├── client
    ├── components
    ├── core
    |   ├── application
    |   |   ├── dto
    |   |   ├── mappers
    |   |   └── use-cases
    |   ├── domain
    |   |   ├── entities
    |   |   └── repositories
    |   └── infrastructure
    |       ├── container-registry
    |       ├── errors
    |       ├── midaz
    |       ├── next-auth
    |       └── utils
    ├── context
    ├── core
    ├── exceptions
    ├── helpers
    ├── hooks
    ├── lib
    |   ├── fetcher
    |   ├── intl
    |   ├── languages
    |   ├── theme
    |   └── zod
    ├── providers
    ├── schema
    ├── types
    └── utils

```

#### Public (`./public`)

- `images`: Images used on project.
- `svg`: Svg's used on project.
- `mmongo`, `mpostgres`: Database utilities, including setup and configuration.
- `mpointers`: Explanation of any custom pointer utilities or enhancements used in the project.
- `mzap`: Details on the structured logger adapted for high-performance scenarios.
- `net/http`: Information on HTTP helpers and network communication utilities.
- `shell`: Guide on shell utilities, including scripting and automation tools.

#### SRC (`./src`)

- `app`: Core structure of the Nextjs project, containing UI and API functions
- `client`: http requests from NextJS UI Layer
- `components`: UI project components
- `core`: Business logic for the API nextjs layer
- `context`: Context providers
- `exceptions`: UI Layer exceptions
- `helpers`: Application helpers
- `hooks`: Application Hooks
- `lib`: Configuration/implementation of libs used in the project
- `providers`: Project providers
- `schema`: zod object schemas
- `types`: Types is used to mapping UI pages/components data.
- `utils`: Application utils

##### App (`./src/app`)

- `(auth-routes)`: Authentication routes, used in unauthenticated user area
- `(routes)`: Authenticated routes, used in the authenticated user area
- `(api)`: NextJS API Layer, this is a backend NextJS layer.

##### App (`./src/core`)

- `(application)`: contain application layer in clean architecture
- `(domain)`: contain domain layer in clean architecture
- `(infrastructure)`: contain infrastructure layer in clean architecture
