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
