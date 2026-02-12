## [discovery-service 1.10.14](https://github.com/siderolabs/discovery-service/releases/tag/v1.10.14) (2026-02-12)

Welcome to the v1.10.14 release of discovery-service!



Please try out the release binaries and report any issues at
https://github.com/siderolabs/discovery-service/issues.

### Contributors

* Andrey Smirnov

### Changes
<details><summary>1 commit</summary>
<p>

* [`e0c8062`](https://github.com/siderolabs/discovery-service/commit/e0c8062eeadfceffd11575c88989b9fd65f0cafb) chore: rekres and update dependencies
</p>
</details>

### Dependency Changes

* **github.com/siderolabs/discovery-api**     v0.1.6 -> v0.1.8
* **github.com/siderolabs/discovery-client**  v0.1.13 -> v0.1.15
* **golang.org/x/net**                        v0.47.0 -> v0.50.0
* **golang.org/x/sync**                       v0.18.0 -> v0.19.0
* **google.golang.org/grpc**                  v1.77.0 -> v1.78.0
* **google.golang.org/protobuf**              v1.36.10 -> v1.36.11

Previous release can be found at [v1.0.13](https://github.com/siderolabs/discovery-service/releases/tag/v1.0.13)

## [discovery-service 1.0.13](https://github.com/siderolabs/discovery-service/releases/tag/v1.0.13) (2025-12-10)

Welcome to the v1.0.13 release of discovery-service!



Please try out the release binaries and report any issues at
https://github.com/siderolabs/discovery-service/issues.

### Contributors

* Utku Ozdemir

### Changes
<details><summary>1 commit</summary>
<p>

* [`1d3ea34`](https://github.com/siderolabs/discovery-service/commit/1d3ea3400035de533028903e5dcaadfda872297e) feat: add support for custom persistent snapshot store
</p>
</details>

### Dependency Changes

This release has no dependency changes

Previous release can be found at [v1.0.12](https://github.com/siderolabs/discovery-service/releases/tag/v1.0.12)

## [discovery-service 1.0.12](https://github.com/siderolabs/discovery-service/releases/tag/v1.0.12) (2025-11-28)

Welcome to the v1.0.12 release of discovery-service!



Please try out the release binaries and report any issues at
https://github.com/siderolabs/discovery-service/issues.

### Contributors

* Andrey Smirnov

### Changes
<details><summary>2 commits</summary>
<p>

* [`b7b68e0`](https://github.com/siderolabs/discovery-service/commit/b7b68e021747d73608a9f622e9ba581e3cf1e1ea) chore: update dependencies, Go version
* [`2c1239f`](https://github.com/siderolabs/discovery-service/commit/2c1239f89dab4e2b9a7c5555aef76cca1ba8fca9) refactor: use DynamicCertificate from crypto library
</p>
</details>

### Dependency Changes

* **github.com/grpc-ecosystem/go-grpc-middleware/v2**  v2.3.2 -> v2.3.3
* **github.com/planetscale/vtprotobuf**                6f2963f01587 -> ba97887b0a25
* **github.com/prometheus/client_golang**              v1.22.0 -> v1.23.2
* **github.com/siderolabs/crypto**                     v0.6.4 **_new_**
* **github.com/siderolabs/discovery-client**           v0.1.12 -> v0.1.13
* **github.com/siderolabs/gen**                        v0.8.4 -> v0.8.6
* **github.com/siderolabs/go-debug**                   v0.5.0 -> v0.6.1
* **github.com/siderolabs/proto-codec**                v0.1.2 -> v0.1.3
* **github.com/stretchr/testify**                      v1.10.0 -> v1.11.1
* **go.uber.org/zap**                                  v1.27.0 -> v1.27.1
* **golang.org/x/net**                                 v0.41.0 -> v0.47.0
* **golang.org/x/sync**                                v0.15.0 -> v0.18.0
* **golang.org/x/time**                                v0.12.0 -> v0.14.0
* **google.golang.org/grpc**                           v1.73.0 -> v1.77.0
* **google.golang.org/protobuf**                       v1.36.6 -> v1.36.10

Previous release can be found at [v1.0.11](https://github.com/siderolabs/discovery-service/releases/tag/v1.0.11)

## [discovery-service 1.0.11](https://github.com/siderolabs/discovery-service/releases/tag/v1.0.11) (2025-07-03)

Welcome to the v1.0.11 release of discovery-service!



Please try out the release binaries and report any issues at
https://github.com/siderolabs/discovery-service/issues.

### Contributors

* Andrey Smirnov

### Changes
<details><summary>1 commit</summary>
<p>

* [`01e232a`](https://github.com/siderolabs/discovery-service/commit/01e232adc32b18d51e66fe25e6876dff7bf0ccfb) fix: pull in new client for FIPS-140-3 compliance
</p>
</details>

### Dependency Changes

* **github.com/fsnotify/fsnotify**                                       v1.8.0 -> v1.9.0
* **github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus**  v1.0.1 -> v1.1.0
* **github.com/grpc-ecosystem/go-grpc-middleware/v2**                    v2.3.1 -> v2.3.2
* **github.com/prometheus/client_golang**                                v1.21.1 -> v1.22.0
* **github.com/siderolabs/discovery-client**                             v0.1.11 -> v0.1.12
* **github.com/siderolabs/gen**                                          v0.8.0 -> v0.8.4
* **golang.org/x/net**                                                   v0.37.0 -> v0.41.0
* **golang.org/x/sync**                                                  v0.12.0 -> v0.15.0
* **golang.org/x/time**                                                  v0.11.0 -> v0.12.0
* **google.golang.org/grpc**                                             v1.71.0 -> v1.73.0
* **google.golang.org/protobuf**                                         v1.36.5 -> v1.36.6

Previous release can be found at [v1.0.10](https://github.com/siderolabs/discovery-service/releases/tag/v1.0.10)

## [discovery-service 1.0.10](https://github.com/siderolabs/discovery-service/releases/tag/v1.0.10) (2025-03-14)

Welcome to the v1.0.10 release of discovery-service!



Please try out the release binaries and report any issues at
https://github.com/siderolabs/discovery-service/issues.

### Contributors

* Andrey Smirnov
* Dmitriy Matrenichev
* Utku Ozdemir

### Changes
<details><summary>16 commits</summary>
<p>

* [`6a44f8c`](https://github.com/siderolabs/discovery-service/commit/6a44f8c89b3bd127978b7ab17f17b1bff2d9f5dd) chore: bump dependencies
* [`761d53a`](https://github.com/siderolabs/discovery-service/commit/761d53a418d75438529293da808491774a5104e2) feat: update dependencies
* [`7c1129e`](https://github.com/siderolabs/discovery-service/commit/7c1129e3e77a3e19e00386a4e00f8bfae5043abe) chore: bump deps
* [`2bb245a`](https://github.com/siderolabs/discovery-service/commit/2bb245aa38c1d59b671d5fb25b6fa802f408c521) fix: do not register storage metric collectors if it is not enabled
* [`b8da986`](https://github.com/siderolabs/discovery-service/commit/b8da986b5ab4acf029df40f0116d1f020c370a3e) fix: reduce memory allocations (logger)
* [`3367c7b`](https://github.com/siderolabs/discovery-service/commit/3367c7b34912ac742dd6fe8e3fe758f61225cddf) chore: add proto-codec/codec
* [`efbb10b`](https://github.com/siderolabs/discovery-service/commit/efbb10bdfd3c027c5c1942b34e1b803d8f8fa10a) fix: properly parse peer address
* [`cf39974`](https://github.com/siderolabs/discovery-service/commit/cf39974104bbfc291289736847cf05e3a205301e) feat: support direct TLS serving
* [`270f257`](https://github.com/siderolabs/discovery-service/commit/270f2575e71bc0ade00d1c58c2787c01d285dd74) chore: bump deps
* [`74bca2d`](https://github.com/siderolabs/discovery-service/commit/74bca2da5cc86fd6efc57838b69a89beb2149069) feat: export service entrypoint
* [`86e1317`](https://github.com/siderolabs/discovery-service/commit/86e131779aba903276f3762ee9330eed868aabbe) feat: log the state file size on load and save
* [`417251c`](https://github.com/siderolabs/discovery-service/commit/417251c0ba82917a5d31e9bae56eb054340a0c64) fix: fix the panic in loading state from storage
* [`10c83d2`](https://github.com/siderolabs/discovery-service/commit/10c83d2eab0338fc979f84bc9e9fade7a1b45fed) release(v1.0.1): prepare release
* [`196c609`](https://github.com/siderolabs/discovery-service/commit/196c609d1ed72e4ca16fddb0884f1a5b2ecb7338) fix: use shared gRPC buffers, lower buffer size
* [`a2217bd`](https://github.com/siderolabs/discovery-service/commit/a2217bd298cb85ccfff74bc5313ccacfdace9d75) chore: migrate from wrapped sync.Pool to HashTrieMap
* [`8a7a0d4`](https://github.com/siderolabs/discovery-service/commit/8a7a0d4a43950894ed35b3d3498e4e84014ed4b0) chore: bump deps
</p>
</details>

### Changes since v1.0.9
<details><summary>2 commits</summary>
<p>

* [`6a44f8c`](https://github.com/siderolabs/discovery-service/commit/6a44f8c89b3bd127978b7ab17f17b1bff2d9f5dd) chore: bump dependencies
* [`761d53a`](https://github.com/siderolabs/discovery-service/commit/761d53a418d75438529293da808491774a5104e2) feat: update dependencies
</p>
</details>

### Dependency Changes

* **github.com/fsnotify/fsnotify**                                       v1.8.0 **_new_**
* **github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus**  v1.0.0 -> v1.0.1
* **github.com/grpc-ecosystem/go-grpc-middleware/v2**                    v2.0.1 -> v2.3.1
* **github.com/jonboulle/clockwork**                                     fc59783b0293 -> v0.5.0
* **github.com/planetscale/vtprotobuf**                                  v0.6.0 -> 6f2963f01587
* **github.com/prometheus/client_golang**                                v1.19.0 -> v1.21.1
* **github.com/siderolabs/discovery-api**                                v0.1.4 -> v0.1.6
* **github.com/siderolabs/discovery-client**                             v0.1.8 -> v0.1.11
* **github.com/siderolabs/gen**                                          v0.4.8 -> v0.8.0
* **github.com/siderolabs/go-debug**                                     v0.3.0 -> v0.5.0
* **github.com/siderolabs/proto-codec**                                  v0.1.2 **_new_**
* **github.com/stretchr/testify**                                        v1.9.0 -> v1.10.0
* **golang.org/x/net**                                                   v0.37.0 **_new_**
* **golang.org/x/sync**                                                  v0.6.0 -> v0.12.0
* **golang.org/x/time**                                                  v0.5.0 -> v0.11.0
* **google.golang.org/grpc**                                             v1.62.1 -> v1.71.0
* **google.golang.org/protobuf**                                         v1.34.1 -> v1.36.5

Previous release can be found at [v1.0.0](https://github.com/siderolabs/discovery-service/releases/tag/v1.0.0)

## [discovery-service 1.0.1](https://github.com/siderolabs/discovery-service/releases/tag/v1.0.1) (2024-05-28)

Welcome to the v1.0.1 release of discovery-service!



Please try out the release binaries and report any issues at
https://github.com/siderolabs/discovery-service/issues.

### Contributors

* Dmitriy Matrenichev
* Andrey Smirnov

### Changes
<details><summary>3 commits</summary>
<p>

* [`196c609`](https://github.com/siderolabs/discovery-service/commit/196c609d1ed72e4ca16fddb0884f1a5b2ecb7338) fix: use shared gRPC buffers, lower buffer size
* [`a2217bd`](https://github.com/siderolabs/discovery-service/commit/a2217bd298cb85ccfff74bc5313ccacfdace9d75) chore: migrate from wrapped sync.Pool to HashTrieMap
* [`8a7a0d4`](https://github.com/siderolabs/discovery-service/commit/8a7a0d4a43950894ed35b3d3498e4e84014ed4b0) chore: bump deps
</p>
</details>

### Dependency Changes

* **github.com/siderolabs/gen**  v0.4.8 -> v0.5.0

Previous release can be found at [v1.0.0](https://github.com/siderolabs/discovery-service/releases/tag/v1.0.0)

