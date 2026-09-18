# Changelog

## 0.1.0-alpha.0 (2026-09-18)


### Features

* agent execution via containerized pods ([#65](https://github.com/hop-top/tlc/issues/65)) ([27fb845](https://github.com/hop-top/tlc/commit/27fb84584634d46fd7b98ddb3ef37152e93f8140))
* **audit:** external-tool run ledger ([9246730](https://github.com/hop-top/tlc/commit/9246730662ae858189c6e69148158db0c7e97625))
* **auth:** add kit/storage/secret backend; default keyring preserves behaviour ([cb5d1ab](https://github.com/hop-top/tlc/commit/cb5d1ab7a975f98806a8204d2c2740c9acaf32c7))
* **bus:** connect to dpkms cross-process bus hub ([ae206e4](https://github.com/hop-top/tlc/commit/ae206e4af7efbbb6ccb26d452d6d9c7051ff1928))
* **cli:** --format vtodo for task and track export (vtodo) ([9b6c3f2](https://github.com/hop-top/tlc/commit/9b6c3f2f290dfd3abb2406bf479db7448671f4d5))
* **cli:** --rrule flag replaces --remind-every (vtodo) ([1724d42](https://github.com/hop-top/tlc/commit/1724d42bf60d4598493c7a42eb60415ba06abede))
* **cli:** -C/--chdir fuzzy-matches registered projects ([#103](https://github.com/hop-top/tlc/issues/103)) ([814c397](https://github.com/hop-top/tlc/commit/814c39755fe61194a56f9e1e95d9d242d5e66163))
* **cli,displaytime:** humanize temporal columns in table format (T-1384) ([#132](https://github.com/hop-top/tlc/issues/132)) ([e6b630e](https://github.com/hop-top/tlc/commit/e6b630e21faba1d1952201f03c21f3b8adeb5a27))
* **cli:** accept TypeID + alias forms in task subcommands (T-0819) ([b30cd53](https://github.com/hop-top/tlc/commit/b30cd53019a87f8f361d45cb75866ae9e7756836))
* **cli:** accept TypeID + slug forms in track subcommands (T-0820) ([84bf046](https://github.com/hop-top/tlc/commit/84bf0462a4f0abbe2d29b1c968e8c2a39e2eb6f6))
* **cli:** add --amend to task update (T-0865) ([#118](https://github.com/hop-top/tlc/issues/118)) ([e2c0c1a](https://github.com/hop-top/tlc/commit/e2c0c1a73231239e85816c0d6a9d32a0f5bb93e5))
* **cli:** add --note|-n to state-changing task subcommands (T-1178) ([4c51466](https://github.com/hop-top/tlc/commit/4c51466c56639081f6a8e8d92f7d364058605404))
* **cli:** add --offline, --profile, --instance global flags ([6d18c6f](https://github.com/hop-top/tlc/commit/6d18c6f8fef973e99db3cd59bd464e0ff3d2f2f0))
* **cli:** add --priority and --blocked-by filters to task list ([cf8a6e9](https://github.com/hop-top/tlc/commit/cf8a6e906a2ab30f4ff50a449277c41b3e03d01c))
* **cli:** add --priority and --blocked-by filters to task list ([3e97742](https://github.com/hop-top/tlc/commit/3e977425fb4a0381734c6c30a1c8a5603e85b008))
* **cli:** add project init alias, project prune, and init test isolation ([803270f](https://github.com/hop-top/tlc/commit/803270f2c7a27032e2f2b6dacbce9942525ff7fc))
* **cli:** add serve subcommand for HTTP API + event bus access ([#150](https://github.com/hop-top/tlc/issues/150)) ([cdbcc52](https://github.com/hop-top/tlc/commit/cdbcc52bd815322872230aac8e37c021ce403bd2))
* **cli:** add task graph command for dependency visualization ([7de746c](https://github.com/hop-top/tlc/commit/7de746c4dc51a50db80be10d9c841da720f033b4))
* **cli:** add temporal filters to task list (T-0908) ([#127](https://github.com/hop-top/tlc/issues/127)) ([a2dddd6](https://github.com/hop-top/tlc/commit/a2dddd62874e85d10e54206407138723e8545852))
* **cli:** adopt hop.top/kit for charm v2 migration ([95416aa](https://github.com/hop-top/tlc/commit/95416aa951399ccdea91a83edc1c02a406e759e5))
* **cli:** adopt hop.top/kit for charm v2 migration ([be1fd74](https://github.com/hop-top/tlc/commit/be1fd742d13ea689cbd2db46d449476c2eefdd6f))
* **cli:** bind temporal display to kit LoadTimezone/FormatInZone (T-0907) ([#126](https://github.com/hop-top/tlc/issues/126)) ([e66a736](https://github.com/hop-top/tlc/commit/e66a73682b7af88825b8daeb84eb1835ea2d72e0))
* **cli:** case-insensitive + alias + fuzzy matching for status/priority filters ([efc775a](https://github.com/hop-top/tlc/commit/efc775a515c30bad76a614107ae8875ed4cf3bbe))
* **cli:** config-driven task effort vocabulary ([bf1062c](https://github.com/hop-top/tlc/commit/bf1062c0081ce3a0a04f57c882dce735765263e8))
* **cli:** config-driven task priority vocabulary ([366db79](https://github.com/hop-top/tlc/commit/366db79ea4a614343467226a3445416131a2e2fa))
* **cli:** config-driven task status vocabulary ([ce97791](https://github.com/hop-top/tlc/commit/ce977918ca33da61e2f89730df49fcef1245ba4d))
* **cli:** configurable list columns + per-command config defaults ([#151](https://github.com/hop-top/tlc/issues/151)) ([59b04e9](https://github.com/hop-top/tlc/commit/59b04e99fa3783718e354d06eeadf14b1e0bb392))
* **cli:** cross-domain NLP classifier without LLM ([#32](https://github.com/hop-top/tlc/issues/32)) ([b5c7593](https://github.com/hop-top/tlc/commit/b5c75937c17026c8f83a0f3cc5f4fd99b20606f9))
* **cli:** fuzzy project refs + cross-DB task lookup for blocked-by ([d5554b0](https://github.com/hop-top/tlc/commit/d5554b0cf61a484f52207badc56f50d0acd91f10))
* **cli:** group top-level commands via cli.HelpConfig.Groups ([06e7d02](https://github.com/hop-top/tlc/commit/06e7d024e3ed46b1db3ad8b32fd51f3cdfe3b0b3))
* **cli:** interactive config wizard + setup alias ([#35](https://github.com/hop-top/tlc/issues/35)) ([b42ac65](https://github.com/hop-top/tlc/commit/b42ac655d0a22873712ed2e7fc9603a2a3071e17))
* **cli:** migrate alias to kit/console/alias (YAML store) ([3db16b3](https://github.com/hop-top/tlc/commit/3db16b341ba018e41960b98a0da332aaf6226133))
* **cli:** move NL prompt to root cmd (prompt-toplevel) ([#33](https://github.com/hop-top/tlc/issues/33)) ([cba68b4](https://github.com/hop-top/tlc/commit/cba68b4463a3939d253c409daf7f36e54aa3a242))
* **cli:** natural language interface + context dump for task commands ([#31](https://github.com/hop-top/tlc/issues/31)) ([874e6e9](https://github.com/hop-top/tlc/commit/874e6e984cabea2d2d161eff12e00e8884ec8da7))
* **cli:** refactor `tlc task exec` flag surface ([#112](https://github.com/hop-top/tlc/issues/112)) ([ab3f4d7](https://github.com/hop-top/tlc/commit/ab3f4d716946e979b5fca7a90930b920c47a4e9f))
* **cli:** render display alias by default; typeid via -V or machine formats (T-0821) ([8bd072e](https://github.com/hop-top/tlc/commit/8bd072e3986a453eb09bd36b89a7138cfad8bf2a))
* **cli:** rewrite config wizard with huh/charm TUI ([f18770e](https://github.com/hop-top/tlc/commit/f18770eab053c2196dd81608712e0816d3af6d9f))
* **cli:** richer exit codes — 3=not-found, 4=conflict, 5=unauthorized ([#106](https://github.com/hop-top/tlc/issues/106)) ([d0e3ce6](https://github.com/hop-top/tlc/commit/d0e3ce608b5050bf77b7c2b514e7f4274d1b78d1))
* **cli:** surface unmet blockers in task list and show --format json ([597e983](https://github.com/hop-top/tlc/commit/597e983900bd629fd8dd523a27dcd71a0cd43812))
* **cli:** task skip, block and unblock commands ([4cbc911](https://github.com/hop-top/tlc/commit/4cbc9114cf4a0cb47fab958b2709b6348e21b3b7))
* **cli:** themed table output with kit color system ([20ae1e3](https://github.com/hop-top/tlc/commit/20ae1e3dd59d0bd2b6d936b7cf67cb8c0f2a6a02))
* **cli:** themed table output with kit color system ([4f96b69](https://github.com/hop-top/tlc/commit/4f96b69114655791c9aa8d8020575fce58874e64))
* **cli:** validate LLM provider auth after selection in setup wizard ([8f349ac](https://github.com/hop-top/tlc/commit/8f349ac73b72522c0e0b99e76f9fc1b63fa7a4a3))
* **config:** add FilesystemConfig with bool/object YAML unmarshaling ([32ef9f8](https://github.com/hop-top/tlc/commit/32ef9f8f56b900b2f6cf5051a38184177721235f))
* **config:** add InboxConfig to StorageConfig ([a70a9bc](https://github.com/hop-top/tlc/commit/a70a9bc431cc44a6da8a0f5e6ae5c817e3a1e160))
* **config:** adopt shared kit 'config path[s]' subcommands ([d6154fd](https://github.com/hop-top/tlc/commit/d6154fdd791dcd63686239ebd20b46a5d172d422))
* **config:** config-driven task vocabularies and label templates ([1809b5d](https://github.com/hop-top/tlc/commit/1809b5deb13e8bd839de29a062b1eb2a11569e5d))
* **config:** tag policy with composed vocabulary ([db7a3cb](https://github.com/hop-top/tlc/commit/db7a3cb4ef5d978e012f7367e6a41b0e20a05ab7))
* **core,cli:** add DueAt to Track model (T-0904) ([#125](https://github.com/hop-top/tlc/issues/125)) ([b20c495](https://github.com/hop-top/tlc/commit/b20c49562c11fd386036b2a20051466adc33115e))
* **core:** add Task.Seq + Track.Slug; rename ValidateTrackID -&gt; ValidateTrackSlug (T-0814) ([085563f](https://github.com/hop-top/tlc/commit/085563f19ed50b04b5bceab630622bc75a43b744))
* **core:** cross-track blocked-by refs in plan ingestion (T-0434) ([#46](https://github.com/hop-top/tlc/issues/46)) ([953f420](https://github.com/hop-top/tlc/commit/953f420aa658873ee3dd73d26c402a09da49e9c4))
* **core:** ParseTaskRef + ParseTrackRef helpers (T-0818) ([7d7b762](https://github.com/hop-top/tlc/commit/7d7b762736ac03f226c7778b4e709c43c5a828bf))
* **core:** per-tag workflow overrides govern transitions ([295b35b](https://github.com/hop-top/tlc/commit/295b35b374ba8f31c9be847c827f423ddae2f664))
* **core:** plan/flow paths emit TypeIDs; logs persist TypeID TaskID (T-0816, T-0817, T-0824) ([ec84b6f](https://github.com/hop-top/tlc/commit/ec84b6f9da3297e5fbc00ad22aef126cee9c9cdd))
* **core:** replace Task.RemindEvery with Task.RRule (vtodo) ([e13dd64](https://github.com/hop-top/tlc/commit/e13dd64af10f687a9922b3f02c1e2a19494f1aca))
* **core:** two-phase cross-track plan ingestion (T-0435) ([#47](https://github.com/hop-top/tlc/issues/47)) ([2ffd757](https://github.com/hop-top/tlc/commit/2ffd7578ecbe03f9e0e6ef8604e554bf82c6af8e))
* **core:** TypeID generation helpers (T-0810, T-0811) ([269974d](https://github.com/hop-top/tlc/commit/269974d4322cf2aff2d2852b9d260fb985b9d19d))
* dependency graph + execution strategy for tracks ([#75](https://github.com/hop-top/tlc/issues/75)) ([802abb3](https://github.com/hop-top/tlc/commit/802abb317281e3a16e9c695f3cb0f79344fd7807))
* **events:** add kit/bus typed topics and domain payload structs ([336caac](https://github.com/hop-top/tlc/commit/336caac13a8bcd3849e38d1165ffb79db7cbd167))
* **events:** adopt kit/bus for audit + cross-tool eventing ([#56](https://github.com/hop-top/tlc/issues/56)) ([0425754](https://github.com/hop-top/tlc/commit/042575426a6487689265d7c00b94581fca360975))
* **events:** per-entity state-machine transition topics (T-1127) ([b8886f2](https://github.com/hop-top/tlc/commit/b8886f243a231dfff664b7774228f078cf065b8b))
* **events:** publish tlc.task.* / tlc.track.* / tlc.flow.* via WithTopics ([e7eb09f](https://github.com/hop-top/tlc/commit/e7eb09f274dadb1dab13dfaf5d1fcf02072515be))
* filesystem projection for tasks ([fd17b9d](https://github.com/hop-top/tlc/commit/fd17b9d425fe89e3700ccf7f1613e30d402384b5))
* flow use cases — 15 stories + fixtures + e2e tests ([#77](https://github.com/hop-top/tlc/issues/77)) ([b34a4a1](https://github.com/hop-top/tlc/commit/b34a4a1bfd5f55359950c69e403ea93f290d9c3f))
* **flow:** add track-ship end-to-end delivery flow ([ffb3762](https://github.com/hop-top/tlc/commit/ffb3762e398ccaeafb838876c85119f9473b5051))
* **flow:** aggregate predecessor outputs in join step (T-0751) ([9c40184](https://github.com/hop-top/tlc/commit/9c40184b8d5b6cd3acbeb6920406ef3e31661413))
* **flow:** honor condition: field on task steps (T-0755) ([b693590](https://github.com/hop-top/tlc/commit/b6935908123012c296b5e79cfebd9d2eda5a7176))
* **flowtest/shims:** tool shims for sandboxed flow execution ([d2debf7](https://github.com/hop-top/tlc/commit/d2debf768a4f9804144e88e50367b4d4a34ca059))
* **flowtest:** adapter Probe/Operation/BuildArgs + SandboxAgentRunner wiring ([8369d89](https://github.com/hop-top/tlc/commit/8369d8905d73161ffe37e9412e26a288e25d0d37))
* **flowtest:** add framework adapters (LangChain, GoogleADK, Mastra, Ollama, OpenAI, N8N, Bedrock) ([f4301ed](https://github.com/hop-top/tlc/commit/f4301ed7ce130e207a6d18812318cec9e44e845c))
* **flowtest:** agent dispatch via SandboxAgentRunner (T-0158–T-0161) ([ad13abe](https://github.com/hop-top/tlc/commit/ad13abe33b9235e6c28b8b9a9b11b0809623cd73))
* **flowtest:** deterministic flow test runner with cassette record/replay ([0d8a10c](https://github.com/hop-top/tlc/commit/0d8a10c789fb80f910b079e92c48c6159afac561))
* **flowtest:** fixtures, eva contracts + integration tests ([163abc5](https://github.com/hop-top/tlc/commit/163abc51979d4578d480d0dcd5e847778e3d0ce6))
* **flowtest:** implement ExecAgentRunner for type=exec steps (T-0729) ([fdb73b4](https://github.com/hop-top/tlc/commit/fdb73b454460111acfd7b68ec8609d4366b7fd9b))
* **flowtest:** pluggable AgentAdapter + AdapterResolver + tools routing ([64d5568](https://github.com/hop-top/tlc/commit/64d5568daf13e5545c4f253a992acb65dc4b9879))
* **flowtest:** shims, sandbox, run, hook, embed scaffold ([cb9aa8d](https://github.com/hop-top/tlc/commit/cb9aa8de1f4ad9987841dd1fa8cd4966caf225b5))
* **flowtest:** warn when contract present but EVA_URL unset (T-0733) ([a1655b7](https://github.com/hop-top/tlc/commit/a1655b7e21ab2cd6bee61bc9b9b0fdbb1042d619))
* **flow:** triggers.cron field + scheduler with missed-tick + concurrency policies ([ca3ba42](https://github.com/hop-top/tlc/commit/ca3ba42664c5c8784a80a7b9d50c7f1676729358))
* **flow:** type: human step with approve/reject/timeout/capability gate ([5be4bce](https://github.com/hop-top/tlc/commit/5be4bce5f5511d505435f6054c9577dad00cd39b))
* **flow:** wire --var inputs and {{var}} substitution ([#39](https://github.com/hop-top/tlc/issues/39)) ([3b3630f](https://github.com/hop-top/tlc/commit/3b3630fa22fb047909060767f25d3a9af19fc1d6))
* group task list output by track, assignee, tag and more ([#173](https://github.com/hop-top/tlc/issues/173)) ([2ec818a](https://github.com/hop-top/tlc/commit/2ec818ac9c1d50a954781646f2a4b985e761dae1))
* **inbox:** add `tlc inbox process` command + auto_process hook ([a24fc7d](https://github.com/hop-top/tlc/commit/a24fc7d90111a41c2f1f484a99b7e5d92c85ee3e))
* **inbox:** add InboxProcessor interface + FileInboxProcessor ([c579a83](https://github.com/hop-top/tlc/commit/c579a833be87fcd6b384e54f439c4176e11f81d4))
* **inbox:** add JSON + Markdown inbox parsers ([5b4e36c](https://github.com/hop-top/tlc/commit/5b4e36c61510e665ed7a308c6da2b54dbca36f79))
* **inbox:** file-based inbox protocol for task creation and transitions ([ced982f](https://github.com/hop-top/tlc/commit/ced982fb9b3fbf4a430be7ea50ebbb230913dc12))
* **init:** add tasks/ to .tlc/.gitignore ([e4bd0e4](https://github.com/hop-top/tlc/commit/e4bd0e411f59942b240e0535e0167760059b811a))
* **labels:** generate priority, effort and status axes from config ([25bf97e](https://github.com/hop-top/tlc/commit/25bf97e62b28eea6697454a961cacbaccd2afb72))
* **labels:** label init seeds domain vocabulary into config ([5a7fda9](https://github.com/hop-top/tlc/commit/5a7fda960bcc3fa697ca73a37dcdef7ffe41e8a6))
* **labels:** node-backend, library, monorepo and infra templates ([319eeb8](https://github.com/hop-top/tlc/commit/319eeb8bc2210a7ab4ba84f5c03a9934d3e31d27))
* **plugins:** mappers preserve tlc TypeID across sync round-trip (T-0823) ([0d29958](https://github.com/hop-top/tlc/commit/0d299584fbcc327573000f5330ae43831325b9b5))
* **plugins:** vtodo-sync plugin (file mode) (vtodo) ([df2bfa3](https://github.com/hop-top/tlc/commit/df2bfa3e8d1e20fa5de031514b4fc5cdef54c299))
* **policy:** adopt kit/runtime/policy with delete-requires-note default (T-1192) ([0ca0df9](https://github.com/hop-top/tlc/commit/0ca0df9304a46ea2e1e2e20bbe62c63b2df4968f))
* recipes replace flows ([#176](https://github.com/hop-top/tlc/issues/176)) ([58c89ac](https://github.com/hop-top/tlc/commit/58c89acd993bc50b5aaf5984e439db685d59d310))
* short track IDs and bounded slugs ([#172](https://github.com/hop-top/tlc/issues/172)) ([9d4b833](https://github.com/hop-top/tlc/commit/9d4b83380756f81b19a2a67c940e19bd76a92f44))
* **storage,build:** auto-migrate statuses + Makefile prebuild target ([#91](https://github.com/hop-top/tlc/issues/91)) ([a0c130e](https://github.com/hop-top/tlc/commit/a0c130e044574255d43aa5c1ee5dec86d745dab9))
* **storage:** add Projector interface + FilesystemProjector ([b100184](https://github.com/hop-top/tlc/commit/b10018441bb565ee4c33b7a89d3f92a182d5ac7d))
* **storage:** atomic seq allocation in task INSERT path (T-0815) ([02ff4a5](https://github.com/hop-top/tlc/commit/02ff4a5f3dd73abaee94847bccc4d01696f85380))
* **storage:** backup DB before running pending migrations ([d48302a](https://github.com/hop-top/tlc/commit/d48302ace22fe6072a2b37c29b2aacefbbac238f))
* **storage:** migration v13 — typeid PKs + seq/slug aliases (T-0812, T-0813) ([326bef2](https://github.com/hop-top/tlc/commit/326bef2307dbe5f69b9be35093f625f84235bc5c))
* **storage:** wire Projector into SQLiteStorage post-commit ([bff3bf4](https://github.com/hop-top/tlc/commit/bff3bf4e155c1e248b86849e640a635b49e742a5))
* **storage:** write DB backups to &lt;dbDir&gt;/.dbs/ + migrate existing ([6270f47](https://github.com/hop-top/tlc/commit/6270f47e636fd8ad848b641bfab59a741b6b45eb))
* **sync:** add Bitbucket sync adapter plugin ([#20](https://github.com/hop-top/tlc/issues/20)) ([0d72cd3](https://github.com/hop-top/tlc/commit/0d72cd3b41d2da7a07b339cb8bd3d69b7399e53a))
* **sync:** add Gitea/Forgejo sync adapter plugin ([#22](https://github.com/hop-top/tlc/issues/22)) ([91a7d7c](https://github.com/hop-top/tlc/commit/91a7d7c51338c6df031c6995c69d12dd1fc64225))
* **sync:** add GitLab sync adapter plugin ([#19](https://github.com/hop-top/tlc/issues/19)) ([749b144](https://github.com/hop-top/tlc/commit/749b144b5ad852da6e118e278265465dacea9def))
* **sync:** carry effective task vocabulary over the plugin protocol ([2beea9f](https://github.com/hop-top/tlc/commit/2beea9fee87302eacdc7fcc772775369f1063367))
* **task:** add due dates, reminders, recurrence, and auto-scheduling ([25efd73](https://github.com/hop-top/tlc/commit/25efd73569073b6bf604856456eb677f4a22c4cf))
* **task:** add eva field and difficulty tier support ([c80cf2a](https://github.com/hop-top/tlc/commit/c80cf2a7f227e2f8228f949c3d17e02c7b32d072))
* **task:** config-driven priority derivation ([416b25d](https://github.com/hop-top/tlc/commit/416b25deddc3dc913c6114698508ad12ca3e00a4))
* **tasks:** add `tlc tasks sync` command + projection wiring + E2E tests ([cf461e8](https://github.com/hop-top/tlc/commit/cf461e82ab9b64a290d9fc2f0f09a3b94a54693e))
* **task:** show accepts multiple IDs (T-0196) ([2e906b3](https://github.com/hop-top/tlc/commit/2e906b31dc57867996901e75eb5745f13d065ecd))
* **tlc:** adopt kit/core/stage — gate track/task creation by scope Stage (T-1313) ([#109](https://github.com/hop-top/tlc/issues/109)) ([0254504](https://github.com/hop-top/tlc/commit/025450418a895246b2e683bf1b7aa76d501402d1))
* **tlc:** agents.yaml wiring for flow run dispatch (T-0198) ([e9215ec](https://github.com/hop-top/tlc/commit/e9215ec4458749782ac36165eed4c8388157b880))
* **tlc:** flow approve|reject|cancel CLI subcommands (T-0199) ([a36d138](https://github.com/hop-top/tlc/commit/a36d13824a9c7a178bd8569e991c4c2d2a8ac1c3))
* **track:** add track registry for work stream management ([c657590](https://github.com/hop-top/tlc/commit/c6575908327362f9efff490399500e29349ee56e))
* **track:** add track registry for work stream management ([45865dc](https://github.com/hop-top/tlc/commit/45865dc4b7b0f4b498da816e6335a091e5b9e6c5))
* **track:** auto-scaffold tracks dir + fix blocked state detection ([b0072ee](https://github.com/hop-top/tlc/commit/b0072eee4911eec13cd584cc0e27cb5fa438943a))
* **track:** expand types + auto-create on missing --track ([#34](https://github.com/hop-top/tlc/issues/34)) ([a3baf52](https://github.com/hop-top/tlc/commit/a3baf5238530e2e872f6d2531c594e252d1d07f4))
* **tracks:** add fuzzy/prefix matching for track ID resolution ([418b093](https://github.com/hop-top/tlc/commit/418b093dd384b48cf7d2fa7af79c9931f1076536))
* **tracks:** fuzzy/prefix matching for track ID resolution ([92d0da9](https://github.com/hop-top/tlc/commit/92d0da96c79f3c2f25004dac8daeff69eb115098))
* **tui:** config-driven status vocabulary seam ([e2229f5](https://github.com/hop-top/tlc/commit/e2229f50ce0f974eac166c17a6cb6311f5c8e23f))
* **tui:** TypeID for storage, alias for display (T-0822) ([6240584](https://github.com/hop-top/tlc/commit/624058493517e89da449971f22f140306e79cac3))
* **ui:** honour ui.theme and ui.table_style ([0348de0](https://github.com/hop-top/tlc/commit/0348de0649f5d0c5a906a91bfd28ec90e61bf3e1))
* **vtodo:** adopt vstar concept mapping for VTODO export and sync ([4a180bf](https://github.com/hop-top/tlc/commit/4a180bfbb1bd4c2cae002b88550615810335abb7))
* **vtodo:** decode VTODO/VJOURNAL into Task/LogEntry (vtodo) ([5745d52](https://github.com/hop-top/tlc/commit/5745d528dc6c10869f251226debe6234815acde4))
* **vtodo:** encode Task/Track/LogEntry to RFC 5545 (vtodo) ([893b7d5](https://github.com/hop-top/tlc/commit/893b7d56d0517edc60ceea0b2182ad39f814c1cf))


### Bug Fixes

* address PR review — dedup, storage scope, error checks ([c983996](https://github.com/hop-top/tlc/commit/c983996ff12fbd0fea1b2fbf78a154b395635670))
* address PR review — project scoping, enum validation, error handling ([57fb093](https://github.com/hop-top/tlc/commit/57fb0930beb671e43d6d58add437b3be7ef895bb))
* address PR review — projector error logging, duplicate ID, config detection, newline guard ([f31e89f](https://github.com/hop-top/tlc/commit/f31e89fb1f29933b48cba0a83e4e7d541bdcede8))
* **build:** build nested plugin modules and fail on plugin build errors ([#159](https://github.com/hop-top/tlc/issues/159)) ([fdfb844](https://github.com/hop-top/tlc/commit/fdfb844d1ef698fdd619f658d706f18ac7f1d41a))
* **build:** clean shim bin dir before rebuild ([6da1507](https://github.com/hop-top/tlc/commit/6da15074a181abb4692d12df46ded38c6b628f56))
* **build:** stop embedding bookkeeping dotfiles as shims ([#167](https://github.com/hop-top/tlc/issues/167)) ([aee64f6](https://github.com/hop-top/tlc/commit/aee64f69054856d63d4a3fb0d6700f3d939f93e5))
* **bus:** replace hardcoded dev token with env-based auth ([051d26f](https://github.com/hop-top/tlc/commit/051d26fc6d77af97c49cf33cdb99f6230a6245ec))
* **ci,test:** address PR [#136](https://github.com/hop-top/tlc/issues/136) code-review feedback (3 minor cleanups) ([77baa54](https://github.com/hop-top/tlc/commit/77baa545e5b764e5dbb5e7817ac91b855b9514de))
* **cli:** -C / --chdir now switches project context (pre-parse os.Args) ([2b9335e](https://github.com/hop-top/tlc/commit/2b9335e98f6ca640f6858b0722e491e2eee1367c))
* **cli:** -c/--config reaches the help flag-enum restamp ([20b5840](https://github.com/hop-top/tlc/commit/20b5840aa366057ec77329015e085ef7eb941c99))
* **cli,storage:** task create no longer mints empty T-NNNN mirror row ([dd74ce4](https://github.com/hop-top/tlc/commit/dd74ce477a8b9caa5adebabbeff215a6e1ffd221))
* **cli:** aggregate counts reflect the full match set ([#157](https://github.com/hop-top/tlc/issues/157)) ([8938ad0](https://github.com/hop-top/tlc/commit/8938ad0655b1f4c631ccc3c4c342ce43e9e70810))
* **cli:** auto-strip deprecated aliases: key from config.yaml after migration ([#104](https://github.com/hop-top/tlc/issues/104)) ([f349da3](https://github.com/hop-top/tlc/commit/f349da36df3bc4dfbe436b46687cbecf930cfb1b))
* **cli:** blocker flags accept display task IDs (T-0620) ([#116](https://github.com/hop-top/tlc/issues/116)) ([8e6183e](https://github.com/hop-top/tlc/commit/8e6183e762622f71cbfe5222421833a1d0d1be7f))
* **cli:** case-insensitive create + update for status/priority/effort ([#114](https://github.com/hop-top/tlc/issues/114)) ([d099475](https://github.com/hop-top/tlc/commit/d09947583a4bc9d61a1904d28e625e60b2031553))
* **cli:** case-insensitive T-NNNN task ID matching ([56fc9f1](https://github.com/hop-top/tlc/commit/56fc9f1d2b9261e2f17505690d848deea2311a58))
* **cli:** config-driven vocabularies in interactive create form ([18016f5](https://github.com/hop-top/tlc/commit/18016f558b1a0a4bc6a2e074ddf9159efd230bc0))
* **cli:** decouple typeid display from --verbose; adopt kit -c/--config ([#110](https://github.com/hop-top/tlc/issues/110)) ([b1dc473](https://github.com/hop-top/tlc/commit/b1dc473709044fa9feb15ac30a8b3cc91f46310f))
* **cli:** derive default status filter from vocabulary roles ([47bc978](https://github.com/hop-top/tlc/commit/47bc978a24941b96edc2116545cae6d8f420d20d))
* **cli:** derive stale, status and tag filters from vocabulary roles ([6c7cd4a](https://github.com/hop-top/tlc/commit/6c7cd4a334dd97c920bf40f56a228e55bfd9a9ff))
* **cli:** drop duplicate --verbose flag after kit upgrade ([9f4eee0](https://github.com/hop-top/tlc/commit/9f4eee04aa3a9fabdbc0ac2cf8eaed272740e89c))
* **cli:** drop tlc-owned --verbose flag (kit/console/cli registers it) ([1f0962b](https://github.com/hop-top/tlc/commit/1f0962b3cfd29b81c83dcd2bd1bf4cde8f3e72b1))
* **cli:** flow step --agent-local now actually dispatches the agent (T-0948) ([#121](https://github.com/hop-top/tlc/issues/121)) ([2734594](https://github.com/hop-top/tlc/commit/2734594ace7234ec1d6b5a40e80cfacf4215dec3))
* **cli:** gate all huh dialogs behind shared /dev/tty probe ([a03ad74](https://github.com/hop-top/tlc/commit/a03ad7423b8b9d5e553e3f346f418ac8ce595f02))
* **cli:** gate all huh dialogs behind shared /dev/tty probe ([27417ea](https://github.com/hop-top/tlc/commit/27417eafd4b01b15a9cc191a70ae125de28a4576))
* **cli:** hide typeid in dep renderers and per-row error/log lines ([#111](https://github.com/hop-top/tlc/issues/111)) ([768164f](https://github.com/hop-top/tlc/commit/768164fc00d45347542db1388b70e5730f96e990))
* **cli:** honor --cols/--columns on list commands ([#174](https://github.com/hop-top/tlc/issues/174)) ([a999f8d](https://github.com/hop-top/tlc/commit/a999f8dc6f4d5aefca63e31f6a8380283683cd5c))
* **cli:** honor --dry-run across task lifecycle commands ([ecc3701](https://github.com/hop-top/tlc/commit/ecc37010288b65b165fb753569a1cee0e00b5cc1))
* **cli:** honor --dry-run across track lifecycle verbs ([4c9526a](https://github.com/hop-top/tlc/commit/4c9526a166fbee9472f43f727ad510ee737e2f45))
* **cli:** honor --dry-run in task create without --recipe ([6f3f2d3](https://github.com/hop-top/tlc/commit/6f3f2d32be74dd8596d6c0503091085255b982cb))
* **cli:** honor --dry-run in task delete ([83aec90](https://github.com/hop-top/tlc/commit/83aec905c0520993498a33c87d6cb47934c0e5a9))
* **cli:** inline cobra completer adapter, drop hop.top/uri/completions ([9cdc95d](https://github.com/hop-top/tlc/commit/9cdc95dc285d77356f3190285542caafd053fc1e))
* **cli:** load standalone config under hop mode; refuse ambiguous or repeated init ([d2f1e1f](https://github.com/hop-top/tlc/commit/d2f1e1fb6d383934d6c0126024a75a61f621648e))
* **cli:** log --since/--until flags silently ignored (T-1381) ([#131](https://github.com/hop-top/tlc/issues/131)) ([4e75d8b](https://github.com/hop-top/tlc/commit/4e75d8bfc12defed16ce1d4dc45844a9712e96ab))
* **cli:** make tty gate Windows-aware + close stdout-redirect gaps ([8401189](https://github.com/hop-top/tlc/commit/84011898b2d5788ec22d5dddd7ef24b0e0dbfc1e))
* **cli:** normalize status aliases in task update and unescape markdown in descriptions ([22c84a1](https://github.com/hop-top/tlc/commit/22c84a1e2fb097f1f6e8374dc8c23ddfba89f84f))
* **cli:** per-source aliases migration to prevent scope drift (T-1343) ([#107](https://github.com/hop-top/tlc/issues/107)) ([703f08a](https://github.com/hop-top/tlc/commit/703f08a1e2c4011282073d77083b78323a5a94c4))
* **cli:** persist --note on non-transition task updates ([#153](https://github.com/hop-top/tlc/issues/153)) ([1a35436](https://github.com/hop-top/tlc/commit/1a35436b635ef85243dad99c709563e0fa473c93))
* **cli:** probe /dev/tty before huh confirm dialog (T-0072) ([304be82](https://github.com/hop-top/tlc/commit/304be825f1de9d6cabcc13aaac93eba894c2972b))
* **cli:** projection ingest no longer overwrites DB from stale file (T-1285) ([a2f1dcb](https://github.com/hop-top/tlc/commit/a2f1dcbe173458c3d483bff12750f40564ac7423))
* **cli:** register --verbose/-V as tlc-owned flag (T-0766) ([fa57e9b](https://github.com/hop-top/tlc/commit/fa57e9bae7057da94d0f4d1e6f96381b58577e9c))
* **cli:** rename table options to role-agnostic names ([ea84521](https://github.com/hop-top/tlc/commit/ea845212f380516a018992ca01312c7591205a99))
* **cli:** render empty JSON lists as [] not null ([#170](https://github.com/hop-top/tlc/issues/170)) ([c1b573d](https://github.com/hop-top/tlc/commit/c1b573db5438e12d33dd44528d04f78da321818d))
* **cli:** resolve -c shortname via project registry (T-0437) ([#49](https://github.com/hop-top/tlc/issues/49)) ([257cc54](https://github.com/hop-top/tlc/commit/257cc545607727b6436d71f88e8cf0a309fb272c))
* **cli:** resolve absolute task.todo_file without nesting it ([cc0c332](https://github.com/hop-top/tlc/commit/cc0c3326569cde261e91e35daf73307e8bb88289))
* **cli:** resolve LLM provider via LoadConfig for env var API keys ([ff52f80](https://github.com/hop-top/tlc/commit/ff52f807244b06036e12858b9e6e4821b000df7b))
* **cli:** resolve task display aliases and TypeIDs in tag command ([8d44f7e](https://github.com/hop-top/tlc/commit/8d44f7eb0f9f1cf6b6625298806e73497a727b53))
* **cli:** restore -c &lt;dir&gt; -&gt; &lt;dir&gt;/&lt;localConfig&gt;/config.yaml under kit v0.4 (T-0006) ([be8289c](https://github.com/hop-top/tlc/commit/be8289c8a22a93138078c857c4a06666ddcb8213))
* **cli:** support --track - to filter untracked tasks ([a777ae9](https://github.com/hop-top/tlc/commit/a777ae9a719d04de520ef580dd305f2fe8f88698))
* **cli:** task create honours configured default status ([f8b547f](https://github.com/hop-top/tlc/commit/f8b547fc7ce7c34163e4d920044e587a4c6b7d77))
* **cli:** task show renders BlockedBy/Blocking with display IDs (T-1382) ([#128](https://github.com/hop-top/tlc/issues/128)) ([56969c4](https://github.com/hop-top/tlc/commit/56969c48e83baee5db439cfe6815789a22fa89da))
* **cli:** TLS parser recognises full label vocabulary ([5d2b6df](https://github.com/hop-top/tlc/commit/5d2b6df664b8f833d6fe7dfc41880e1d48160c82))
* **cli:** track update --add-plan honors target track id (T-0857) ([#119](https://github.com/hop-top/tlc/issues/119)) ([57c13af](https://github.com/hop-top/tlc/commit/57c13afd223544cbd49716c5cd50caeba59723df))
* **cli:** unblock annotated write-local, not destructive ([5a16803](https://github.com/hop-top/tlc/commit/5a16803b1104ab14d87eee8f09fc7c66684304bd))
* **cli:** use short refs for same-project blocking/blocked-by (T-0632) ([#79](https://github.com/hop-top/tlc/issues/79)) ([399c21a](https://github.com/hop-top/tlc/commit/399c21a9f447deabf803a4cc5917266be1f84b37))
* **cli:** use TypeID as durable Task.ID, not seq alias ([5193684](https://github.com/hop-top/tlc/commit/5193684f84c9a679a21d1dbc026b45d9c9ac862e))
* **cli:** wrap external errors at file, alias and prompt boundaries ([76d3f24](https://github.com/hop-top/tlc/commit/76d3f244ac7451fc67687eafe99796706a1c43f4))
* **config:** decode snake_case config keys via yaml tag ([35d9504](https://github.com/hop-top/tlc/commit/35d9504c59b54445f8077c869eecb8d449a96052))
* **config:** normalize state-machine keys on every decode path ([24febd5](https://github.com/hop-top/tlc/commit/24febd53014f5d07cdc3d02fcac69f2a80f9826e))
* **config:** reject state machine rules out of terminal statuses ([2effc63](https://github.com/hop-top/tlc/commit/2effc630205cc97c3a876f91e0267cc9f57fd5de))
* **config:** require a hop config before entering hop mode ([6282ff1](https://github.com/hop-top/tlc/commit/6282ff15f965d6635db34474616d7e3ef5501245))
* **config:** resolve project config dir at command entry ([806b10a](https://github.com/hop-top/tlc/commit/806b10a7d665f9d2c8c7c249cd278bba614e29dc))
* **config:** work around kit/xdg StateDir darwin regression (T-0798) ([b0595fb](https://github.com/hop-top/tlc/commit/b0595fb1f9624c4c874c86f92d8890f485f89c4a))
* **core:** admit seeded needs and domain tags under a closed policy ([96084be](https://github.com/hop-top/tlc/commit/96084bef33bc6f6f917b93b1664e88ce762f231c))
* **core:** attribute actor to active aps profile ([#166](https://github.com/hop-top/tlc/issues/166)) ([56cee83](https://github.com/hop-top/tlc/commit/56cee83eb49e5b88505280b5f9e0ff19b8b9c078))
* **core:** honor cross-mode existing config in CreateConfigWithInferredID (T-1303) ([36c112e](https://github.com/hop-top/tlc/commit/36c112efc69e1caffae119fd863af1035597a2f0))
* **core:** include archived tasks in track linkage accounting ([c1bf269](https://github.com/hop-top/tlc/commit/c1bf269971ebcbb9b1c63d5685bc6a0b13b1ade0))
* **core:** include archived tasks in track linkage accounting ([cf6341f](https://github.com/hop-top/tlc/commit/cf6341f00382d452a471cad1947fd154a32d960b))
* **core:** match missing agent config via errors.Is ([9fcbf37](https://github.com/hop-top/tlc/commit/9fcbf371a1d800556a4fa214023a5a2325ccad68))
* **core:** move aps LookPath out of testable cores; skip race flake in CI ([8ac6593](https://github.com/hop-top/tlc/commit/8ac6593edf1c9c2b9e88d3e406d78f59d0352357))
* **core:** plan reconciliation preserves new tasks + keeps unrelated tasks separate (T-0947) ([#120](https://github.com/hop-top/tlc/issues/120)) ([25a70bf](https://github.com/hop-top/tlc/commit/25a70bf177af2f73ba094c8d0cd13ccac3c02028))
* **core:** project detection preserves merged config cascade ([ef2565c](https://github.com/hop-top/tlc/commit/ef2565c16519561b9cce035e3e038aab365763f0))
* **core:** status validator reads effective vocabulary ([042bcc3](https://github.com/hop-top/tlc/commit/042bcc31034a3b8a4fe03a474e130ddb9cb47758))
* **core:** terminality decides age-nudge and reminder exemption ([1bab952](https://github.com/hop-top/tlc/commit/1bab9523fa798dc70920c4b6b755770ea49c2c4c))
* **core:** wire loaded user config into the workflow ([4f04f97](https://github.com/hop-top/tlc/commit/4f04f97c62585829f06908f0ad2538c860bd9d93))
* **core:** wrap external errors at file, webhook and exec boundaries ([ec00f73](https://github.com/hop-top/tlc/commit/ec00f731513cfb4af6804a10c7b0c33784d29096))
* **events:** prefix agent.* topics with tlc. so audit subscriber sees them ([4886484](https://github.com/hop-top/tlc/commit/48864848f115ca20cd566bdcd6f0c9b0458b4d3b))
* **fixtures:** add join steps to composition-child + conditional flows (T-0753) ([c7d1bd8](https://github.com/hop-top/tlc/commit/c7d1bd888b931893256d20c58faa6b04118acdb0))
* **fixtures:** rewrite step_status evaluators to contains ([387e6b9](https://github.com/hop-top/tlc/commit/387e6b95a93424ef89fc260e469184c387ba8a13))
* **flow-parser:** accept depends_on as fallback for join wait_for (T-0754) ([701a37c](https://github.com/hop-top/tlc/commit/701a37cab78731df2f848480ac49b4a8d03f8310))
* **flow:** align exec-cli-smoke example with merged spec + add happy-path fixture ([4667995](https://github.com/hop-top/tlc/commit/46679958f2661282993c81b226182f48be26a7cc))
* **flow:** correct exec cassette replay + passthrough shim build ([#152](https://github.com/hop-top/tlc/issues/152)) ([d1d1a8e](https://github.com/hop-top/tlc/commit/d1d1a8eec4d6822cd3e0b061c79fb073f1fdb0ff))
* **flow:** set ProjectID on tasks created by flow invoke ([#38](https://github.com/hop-top/tlc/issues/38)) ([bb8b869](https://github.com/hop-top/tlc/commit/bb8b8698815c23f318232fdd8ed135bbe3f7252a))
* **flowtest:** add happy-path fixtures for 4 flows ([8f29d2f](https://github.com/hop-top/tlc/commit/8f29d2fecfedc1aa4d4cf0bac48891155b2c4b14))
* **flowtest:** kill exec-runner child process groups via Setpgid (T-0071) ([5ba5274](https://github.com/hop-top/tlc/commit/5ba52744426003ab095b16f2c3144e7a94d46d80))
* **flowtest:** suppress noctx + errcheck on intentional exec-runner sites (T-0071) ([2616598](https://github.com/hop-top/tlc/commit/26165987f9122b538c0f8e18f0adc41bda1e2ce4))
* **gitignore:** scope shim ignore rules to shims/bin/ path (T-0224) ([#69](https://github.com/hop-top/tlc/issues/69)) ([9b27831](https://github.com/hop-top/tlc/commit/9b27831f4c99ffd71783ef930479ccba1abfeb57))
* **go.mod:** pin kit to v0.3.2-patch.3, drop bus-compat replace (T-0748) ([1142477](https://github.com/hop-top/tlc/commit/1142477fad8a5e0cecc55972a89acd0f8b44e6cb))
* hide typeid from default human output (tlc-typeid-display-hygiene) ([#102](https://github.com/hop-top/tlc/issues/102)) ([be8dfa7](https://github.com/hop-top/tlc/commit/be8dfa7c26a2b5bcb6729a9f8bb1e686d56cef87))
* **inbox:** resolve create default status through core ([6363aa3](https://github.com/hop-top/tlc/commit/6363aa3f2794c3b718393c3aac74c60e11d0bee1))
* **inbox:** validate status against configured vocabulary ([a573232](https://github.com/hop-top/tlc/commit/a573232cb644315d171a6b54a96452579ed7c80f))
* **init:** skip auto-detection for init command (GH-1) ([#67](https://github.com/hop-top/tlc/issues/67)) ([38a631e](https://github.com/hop-top/tlc/commit/38a631e99adb848031a04a0b59cc17e2f3331562))
* **internal:** wrap external errors in config, uri, workspace and shims ([dd45c6b](https://github.com/hop-top/tlc/commit/dd45c6b3028b511f2f06b9a76f24a83aa4abcca6))
* **labels:** dedupe generated axes by label name ([083b5f5](https://github.com/hop-top/tlc/commit/083b5f52ecbb9d7019912502315abf4fcf4bab70))
* **labels:** dimension-prefixed type axis, effort axis in templates ([d5efbfc](https://github.com/hop-top/tlc/commit/d5efbfccd8036a83fc4055ffd8b3a73adcfc69ef))
* **labels:** distinct swatches for status:blocked and needs:* ([e90e96d](https://github.com/hop-top/tlc/commit/e90e96d1c8c9b0f2a4666f0c9459d47b6c531ffa))
* **labels:** resolve phantom project types in label templates ([5c8170c](https://github.com/hop-top/tlc/commit/5c8170c549357e63a14fcfe5b447aca3eb918ce1))
* **labels:** restore built-in priority forge swatches ([b52708b](https://github.com/hop-top/tlc/commit/b52708bb4a364c29e758f1ecb10d43c74896ee16))
* **lint:** add sort-results so golangci-lint v1.64.8 loads .golangci.yml ([0edce20](https://github.com/hop-top/tlc/commit/0edce20dfdc43b2d3e14f35aeaa0f47ba0d1f9db))
* **lint:** deprecated pflag field, dead directive, intentional discards ([1d120c7](https://github.com/hop-top/tlc/commit/1d120c73698a1abd6ce89e42cd7e768bc1716f5e))
* **lint:** keep tag-policy rejection message unwrapped ([c4b7d11](https://github.com/hop-top/tlc/commit/c4b7d114c239e5e8ea5abd4fccbae7eeb273c167))
* **lint:** resolve errcheck + nolintlint violations (T-0547) ([#62](https://github.com/hop-top/tlc/issues/62)) ([73ab89c](https://github.com/hop-top/tlc/commit/73ab89ce61906b86ad2a95f4084fe3a1ba6cc0fe))
* **plan-ingest:** resolve blocked-by integers to plan-local task IDs (T-0436) ([#48](https://github.com/hop-top/tlc/issues/48)) ([b4ef277](https://github.com/hop-top/tlc/commit/b4ef277ea032d7fe553ca6bbe199a3d61678fb61))
* **plan:** accept canonical slash cross-project refs, stop dropping them ([821db18](https://github.com/hop-top/tlc/commit/821db18513d6239c9452b38be343a7e692993dfd))
* **plan:** resolve "T-NNNN" blocked-by refs to durable task IDs ([d100f4e](https://github.com/hop-top/tlc/commit/d100f4e7c2043523b16a70740e17fc51a6f3691a))
* **plan:** resolve blocked-by display aliases via seq lookup ([9ad86ec](https://github.com/hop-top/tlc/commit/9ad86ec82eba2f05c7fe4e08df08b54d7a76193a))
* **plan:** resolve blocked-by refs and stop --add-plan wiping manual edges ([3cd5a5f](https://github.com/hop-top/tlc/commit/3cd5a5f6caa2aa976fa52a9a97dd046578b23270))
* **plan:** stop --add-plan wiping blocked-by edges it did not author ([3f5cbb6](https://github.com/hop-top/tlc/commit/3f5cbb656c0c5ffa54e9b502b3f693af9f59220e))
* **plugins:** wrap file errors in vtodo-sync ([cfd74c9](https://github.com/hop-top/tlc/commit/cfd74c920667beb58543df8e56cacdfb54969b31))
* **projection:** skip already-existing task IDs in ingestTODO without warnings (T-1234) ([44c594e](https://github.com/hop-top/tlc/commit/44c594e6e13c7c1cfc411cd6b6cb07b1e2f9ceb1))
* prompt resolution quality + track abandon/list improvements ([#37](https://github.com/hop-top/tlc/issues/37)) ([e1102f7](https://github.com/hop-top/tlc/commit/e1102f7fba2432e50c7419b5535435ffdb762019))
* **prompt:** data-driven synonyms + fix AgeNudge test isolation ([eb470f0](https://github.com/hop-top/tlc/commit/eb470f0c707a977c65e1a1ac3db93056c961dd5b))
* **scheduler:** SKIPPED predecessors satisfy depends_on (T-0756) ([9e36d7e](https://github.com/hop-top/tlc/commit/9e36d7e402ca2849bea6082035516968aa938b3e))
* **schema:** drop task_logs cascade so notes survive task delete (T-1232) ([18a3e29](https://github.com/hop-top/tlc/commit/18a3e29a966c3cd39028ba4eb5f6aa9eb8e5da7c))
* **serve:** HTTP create default status from configured vocabulary ([0cb090c](https://github.com/hop-top/tlc/commit/0cb090c8e3d8a987d999fc032a772142eddcd1db))
* **storage:** break task_logs timestamp ties by id ([#154](https://github.com/hop-top/tlc/issues/154)) ([f1ff264](https://github.com/hop-top/tlc/commit/f1ff264fda60ab409cc476898cea2be54949d67b))
* **storage:** disable FK checks during tracks table rebuild ([099a57e](https://github.com/hop-top/tlc/commit/099a57edb595a2981d12cba92df5df5b82776790))
* **storage:** overdue predicate excludes configured terminal statuses ([42c17ee](https://github.com/hop-top/tlc/commit/42c17ee8440720365a9e3763796840c5a0594294))
* **storage:** rebuild tasks table to drop broken FK on track_id ([2a7a073](https://github.com/hop-top/tlc/commit/2a7a073829d665504523b26d3ab5fd4fa0a97dd2))
* **storage:** rebuild tracks with composite PK ([#36](https://github.com/hop-top/tlc/issues/36)) ([e5c8c31](https://github.com/hop-top/tlc/commit/e5c8c31d93d17f28465e309331ec75d7d88004da))
* **storage:** renumber dup-fix cleanup migration to v17 ([f21e109](https://github.com/hop-top/tlc/commit/f21e1093e0de5436a9073835d25bc534707e73c6))
* **storage:** stop a nil context deadlocking the connection pool ([0c3f479](https://github.com/hop-top/tlc/commit/0c3f479a1685b7c9123cbe31d06504b0b59416d1))
* **storage:** Task temporal write+read normalise to UTC via kit util (T-1383) ([#133](https://github.com/hop-top/tlc/issues/133)) ([c50d4c7](https://github.com/hop-top/tlc/commit/c50d4c7e617247e137878374f199bbb4797a359a))
* **storage:** wrap external errors at filesystem and sql boundaries ([137d27a](https://github.com/hop-top/tlc/commit/137d27a15519a2507a53ea3a1885003ab2843d0f))
* **sync,core:** repo-format + state-drift on task transitions ([#90](https://github.com/hop-top/tlc/issues/90)) ([193f121](https://github.com/hop-top/tlc/commit/193f1218f1a21dea84e94748909953b077ec1ecf))
* **sync:** extract ensureGitHubToken and use in auto-sync push path ([bc9377f](https://github.com/hop-top/tlc/commit/bc9377fefc519756e21d078cbef32c61d7711ec2))
* **sync:** full status, label, and relationship mapping for github-sync ([#18](https://github.com/hop-top/tlc/issues/18)) ([dc6eae3](https://github.com/hop-top/tlc/commit/dc6eae3a4cd1c148df078ca229e62224fa9d6678))
* **sync:** github direction key matches documented spelling ([4dbcbbb](https://github.com/hop-top/tlc/commit/4dbcbbb441951d1ac72e2b34434fdd2537606b85))
* **sync:** route auto-config write through writable config path ([2e35ed6](https://github.com/hop-top/tlc/commit/2e35ed66e403f6c24e07d638c65aa17a6e74797d))
* **task:** push IN_PROGRESS-first sort into SQL before LIMIT (T-0289) ([#72](https://github.com/hop-top/tlc/issues/72)) ([6c4bd95](https://github.com/hop-top/tlc/commit/6c4bd95cbc109d87f3eb56f8a4fc28cf2c020d69))
* **task:** reset flag state between sequential creates (T-0231) ([#66](https://github.com/hop-top/tlc/issues/66)) ([3426267](https://github.com/hop-top/tlc/commit/3426267e9a819d05a69bcc88115f6a5b69462e24))
* **test:** pin TLC_MODE in TestFindAllConfigs (T-1379) ([#123](https://github.com/hop-top/tlc/issues/123)) ([4a1b4e9](https://github.com/hop-top/tlc/commit/4a1b4e9b2d883eb6bb8fa6c452ce2de3de50ebc8))
* **track-list:** --all-projects works from inside project context (T-0765) ([c177edb](https://github.com/hop-top/tlc/commit/c177edbb74e8da571784693506ebc31ac0721f90))
* **track:** allow forward blocked-by refs in plan parser (T-0703) ([73a01d3](https://github.com/hop-top/tlc/commit/73a01d3339f17f189c9f15592a01174eb1b04362))
* **track:** make --add-plan idempotent with plan_mapping reconciliation (T-0615) ([#76](https://github.com/hop-top/tlc/issues/76)) ([24e0887](https://github.com/hop-top/tlc/commit/24e088797f021dc6391a2152844c59159af7025c))
* **track:** preserve multi-segment config-dir prefix in scaffold hints ([46cd0ff](https://github.com/hop-top/tlc/commit/46cd0ffecc7c91b1f61b91fd2b2257bdf675ced1))
* **track:** scope linked task lookup by track project_id ([#40](https://github.com/hop-top/tlc/issues/40)) ([296f2c9](https://github.com/hop-top/tlc/commit/296f2c96762f02ab8efb2169e3cff73596d55057))
* **track:** support cross-project blocked-by refs in plan parser (T-0631) ([#78](https://github.com/hop-top/tlc/issues/78)) ([50ca32b](https://github.com/hop-top/tlc/commit/50ca32ba25dc8c9bb2ee8a0c73ee02d59603241a))
* **tui:** centralize viewport height computation in Update() ([#43](https://github.com/hop-top/tlc/issues/43)) ([25b84e3](https://github.com/hop-top/tlc/commit/25b84e39d83bcfb0706d23f410df238dd64a1579))
* **tui:** kanban columns and grouping follow config ([7e92fc6](https://github.com/hop-top/tlc/commit/7e92fc6fc59225c373a8d37dbf25031cd9558687))
* **tui:** rotate ring and create default follow config ([a0a4c37](https://github.com/hop-top/tlc/commit/a0a4c373694196971f7e84fd2e54e7d398234199))
* **tui:** sort rank and kanban move follow config ([0335145](https://github.com/hop-top/tlc/commit/03351451a5425e913742b777bca27d6039f6a8f1))
* **tui:** status cell reads configured marker ([ec85122](https://github.com/hop-top/tlc/commit/ec85122be133a7cc26c23c3e6ef40015ddcdd885))
* **tui:** task creation UNIQUE constraint on rapid creates (T-0130) ([977000e](https://github.com/hop-top/tlc/commit/977000eb84a28092290316827a41f1c7392612bd))
* **ui:** seed ui.theme default so key search can find it ([978aa90](https://github.com/hop-top/tlc/commit/978aa906450e1aa91aa539cb54a015d495cd68ba))
* **uri:** use project-qualified task references (GH-2) ([#68](https://github.com/hop-top/tlc/issues/68)) ([9daed28](https://github.com/hop-top/tlc/commit/9daed28b40bb45b788ed5f81edf0e0833deea212))
* use filepath.Join for filesystem paths (Windows compat) ([4c49003](https://github.com/hop-top/tlc/commit/4c49003625598797440acb8719e73aee4d7c1e33))
* **vtodo,cli,core:** address Copilot review on PR [#96](https://github.com/hop-top/tlc/issues/96) ([f2935b1](https://github.com/hop-top/tlc/commit/f2935b13987b6bcb806ffb6c1bcb78730cb7b8f5))
* **workflow:** empty state machine rules no longer install built-in ones ([bcfa253](https://github.com/hop-top/tlc/commit/bcfa25350e7e797ed038c12a4c202025c3427d2b))
* **workflow:** match task.workflows override tags case-insensitively ([fda66e8](https://github.com/hop-top/tlc/commit/fda66e88c53af231af9a63c139e00bc300ef316b))
* **workspace:** rank unranked vocabulary values after ranked ones ([e18b5c6](https://github.com/hop-top/tlc/commit/e18b5c66fce22eefaf48819120e1c45101d75558))
* wrap external errors across internal and plugin layers ([0512060](https://github.com/hop-top/tlc/commit/05120607883e407fa3c2faef77acc7426f11d64d))

## Changelog

## Unreleased

### Added

- Root `tlc status` subcommand: read-only project + caller snapshot
  (detected project, caller identity, in-progress/TODO task counts,
  overdue items). Satisfies kit's `checkReservedStatus` shape rule
  and gives users a short health view at the root. (T-1394)
- Kit 12fcc command-surface annotations on every cobra leaf:
  `kit/side-effect` (86 leaves), `kit/idempotent` (61),
  `kit/top-level-verb` (8), plus `Long:` descriptions on 48 leaves
  that previously had none. See
  [`docs/12fcc-conformance-split-plan.md`](docs/12fcc-conformance-split-plan.md)
  for the full contract. (T-1389 — umbrella)
- `--note|-n` flag on `tlc task update` (when `--status` is set) and
  `tlc task delete` (T-1178). Note text is recorded on the
  STATUS_CHANGED / DELETED `task_logs` entry, mirroring the existing
  `tlc task complete --note` pattern.
- `tlc/runtime/policy` adoption: declarative policy YAML at
  `$XDG_CONFIG_HOME/tlc/policies.yaml`, evaluated against state
  changes via kit's policy engine. First boot seeds the file from
  a bundled default; existing user files are never overwritten.
  Default policy: `delete-requires-note`. See
  [`docs/policies.md`](docs/policies.md). (T-1192)

### Changed

- Bumped `hop.top/kit` to the 12fcc-leak branch tip (strict
  command-surface validation: `Root.Validate()` fires at boot and
  rejects unannotated leaves). Boot now fails loud if any leaf is
  missing a required annotation — the new `TestStrictValidationPasses`
  regression test guards this contract in CI. (T-1389)
- Destructive commands (`task delete`, `task unassign`, `task unclaim`,
  `track abandon`, `track archive`, `track delete`, `agent cancel`,
  `flow cancel`, `flow reject`, `project prune`, `alias remove`,
  `auth logout` — 12 in total) now require `--confirm=yes` from kit's
  persistent flag in non-TTY contexts. The
  pre-existing local flags (`--force`, `--yes`, `--no-prompt`) continue
  to work as bridges: when any local skip flag is true the bridge sets
  the inherited `--confirm` to `yes` before the kit gate runs. No
  user-facing CLI change unless scripts run destructive commands
  without any skip flag in a pipe. (T-1392)
- Schema migration v15: `task_logs` rebuilt without
  `ON DELETE CASCADE` on `task_id` so transition notes — including
  delete reasons recorded via `--note` — survive task deletion.
  Migration applies automatically on first run; existing rows are
  preserved. (T-1232)

### Deprecated

- Legacy `aliases:` key in `config.yaml` is now auto-removed after a one-time
  migration to `<config-dir>/aliases.yaml`. The deprecation warning that
  previously fired on every tlc invocation is now a single one-time message;
  on subsequent runs, no warning. No user action required — migration is
  defensive (won't strip the legacy key unless the YAML store has every
  legacy entry first). User-level + project-level configs both covered;
  system config (`/etc/tlc/`) intentionally untouched (root-only). (T-1339)

### Note for operators

- `tlc task delete <id>` without `--note` now exits 4 with a
  `policy "delete-requires-note" denied: ...` error. Update scripts
  to pass `--note "<reason>"`. To restore the pre-T-1192 behavior
  temporarily, edit `$XDG_CONFIG_HOME/tlc/policies.yaml` and remove
  the `delete-requires-note` rule.

### chore
- kit: migrate to role-based hierarchy (`hop.top/kit/go/<role>/<pkg>`).
  All 16 kit packages tlc imports moved from flat to role-based paths
  (e.g. `hop.top/kit/log` → `hop.top/kit/go/console/log`). Replace
  directive in `go.mod` carries the local kit + hdl worktrees until
  upstream tags publish. See `docs/kit-migration-playbook.md` for
  the migration procedure other consumers can reuse. Refs: T-0757,
  T-0794, PR #92.

### feat
- Plan ingestion: two-phase cross-track ref resolution handles
  circular plan references. Phase 1 creates tasks and captures
  any ref whose target track/plan/task is missing in
  `Meta["blocked_by_unresolved"]` instead of hard-failing.
  Phase 2 runs project-wide after every `track update
  --add-plan`, promotes newly resolvable refs into `blocked_by`,
  and rewrites plan.md files whose refs are now all concrete.
  A set of mutually-referencing plans can now be ingested in any
  order without the chicken-and-egg deadlock from T-0434. Hard
  errors (index out of range, title ambiguity) still fail the
  ingest immediately. (T-0435, story 076 scenarios 10-12)
- Plan ingestion: `blocked-by` frontmatter now accepts mixed entries:
  integer indices (intra-track, existing behaviour), concrete
  `"T-NNNN"` task IDs, and `"<track-id>#<N>"` cross-track refs
  (1-based into target track's linked plan). On success the
  source plan.md is rewritten on disk so subsequent ingestions
  read stable `T-NNNN` IDs. (T-0434, story 076)
- Cross-domain NL classifier: track/flow/project prompts now resolve without LLM
  - Keyword tokenizer + vocabulary layer (verbs, nouns, modifiers)
  - Fuzzy noun/verb matching via Levenshtein distance (typo-tolerant, distance ≤2)
  - Confidence scoring: exact match=1.0, distance-1=0.9, distance-2=0.8
  - Aggregate patterns: "count active tracks" → `track list --status active`
  - Pipeline: regex classifier → cross-domain classifier → LLM (LLM only as last resort)

### Added

- **Track registry** — first-class work stream entity grouping tasks with
  lifecycle management, computed health state, and phase progress:
  - `tlc track create/list/show/update/archive/abandon/delete` commands
  - `tlc track summary` project health pulse view
  - Track status machine: pending → active → completed/abandoned → archived
  - Computed state flags: stale, unlinked, blocked, healthy
  - Phase progress from task `phase:N` tags (format: `5/8 (P2/3)`)
  - `--track` flag on `tlc task create/update/list` for linking
  - Auto-transition: pending → active on first linked task claim
  - `--add-plan` flag with frontmatter task extraction and blocked-by
    resolution
  - Configurable plan-extractor command support
  - Cross-project qualified track IDs (`org_repo--track-id`)
  - `--all-projects` flag on `tlc track list`
  - Health thresholds in config: `tracks.stale_threshold`,
    `tracks.health.max_active`, `tracks.health.min_progress_to_start`
  - Overcommit warning when active tracks exceed threshold
- `--blocked <reason>` on `tlc task update`: set blocked reason on a task.
- `--unblock` on `tlc task update`: clear blocked reason.
- `--timeout <duration>` on `tlc task update` and `tlc task create`: set per-task
  stale timeout (e.g. `2h`, `30m`). Resets stale-crossing state on update.
- `effort` field on tasks: set sizing estimate (XS/S/M/L/XL) via
  `tlc task create --effort` or `tlc task update --effort`.
  Shown in `tlc task show`; serialized in JSON/YAML/TLS formats.

### Changed

- `tlc task list` now defaults to showing `IN_PROGRESS` and `TODO` tasks only (instead of all statuses), with `IN_PROGRESS` tasks sorted first. Use `--status` to override.

### Fixed

- config discovery now uses the OS user config directory for user config lookup
- config writes now target the active local config or the user config path; they no
  longer fall back to `/etc/tlc/config.yaml` when no writable config exists
- global TODO ingest now preserves `project_id` metadata and ignores foreign
  same-ID task lines instead of overwriting tasks in the current project
