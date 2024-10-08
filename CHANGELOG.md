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
