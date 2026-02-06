# app-developer (Vultisig)

`app-developer` is a Vultisig plugin that provides developer tooling for creating and submitting new plugin proposals.

It lets external developers:
- create a draft plugin on the production environment (not visible in UI, cannot process user policies)
- manage draft plugin metadata
- generate and sign a draft plugin policy/recipe for review (staging)
- submit proposals for Vultisig team review
- publish an approved plugin after paying the listing fee (e.g. burning VULT), which activates the plugin in production

This repository contains the plugin implementation and any supporting UI/flows required to integrate with the Vultisig developer portal.

## Status
Early development / WIP.

## Related
- Developer Portal: https://developer.vultisig.com