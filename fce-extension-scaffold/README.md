# Safety Incident Log: Confidential Compute Extension (Layer 2)

This is the Layer 2 component of **Safety Incident Log**, built on Flare's
Confidential Compute (FCC) extension scaffold. It runs the AI severity
classification step for workplace safety incident reports inside a Trusted
Execution Environment (TEE), so that the raw report text and the AI API key
never leave hardware-isolated compute.

Built for the Flare Summer Signal hackathon, Bounty 2 (Confidential Compute
Apps). See the top-level project README for the full system (Telegram bot,
severity rubric, Coston2 contract).

## What this extension does

1. Receives the raw text of an incident report as input.
2. Calls an AI model (Gemini) to classify the report: severity tier
   (Low/Medium/High/Critical), driving factor, summary, and justification.
   This call happens inside the TEE, so the API key and the raw text stay
   isolated from the outside world.
3. Computes a Keccak256 hash of the classification result.
4. Returns only the classification data and the hash, plus a flag for
   whether the incident should be logged on-chain (Medium severity or
   higher). The raw report text itself is never returned or persisted
   outside the TEE.
5. If the TEE processing times out or the AI call fails, it falls back to
   a safe default: severity "MEDIUM" with driving factor "near_miss", so
   the safety officer still gets alerted even if the AI is temporarily
   unreachable.

## Status

Deployed and tested end to end on Coston2: contract deployed, TEE
registered, and the classification flow verified against live requests.

## Repository structure

This extension is built on the standard FCC scaffold layout:

```
cmd/main.go                          Extension server entry point
internal/config/config.go            OPType/OPCommand constants
internal/extension/extension.go      Routing + incident classification handler
pkg/types/types.go                   Request/response types for incident data
pkg/types/register.go                Decoder registrations
contracts/InstructionSender.sol      On-chain entry point for this extension
config/                              Env and proxy config (generated/gitignored)
scripts/                             pre-build / post-build / test / full-setup
tools/cmd/                           Deploy, register, and test tooling
```

The customization points (severity classification logic, Gemini API call,
Keccak256 hashing, and the fallback behavior) live in
`internal/extension/extension.go` and `pkg/types/types.go`.

## Prerequisites

- **Go 1.25.1+**
- **Foundry** (`forge`), used to compile the Solidity contract
- **jq**, used to extract ABI/bytecode from Foundry output
- A funded **Coston2 account** for gas (testnet C2FLR from the
  [Coston2 faucet](https://faucet.flare.network/coston2))
- **ngrok** or similar tunneling tool, so the TEE proxy is publicly
  reachable
- Indexer DB credentials for the proxy to fetch signing policies

## Setup and deployment (Coston2)

1. Copy `.env.example` to `.env` and fill in your funded Coston2 private
   key, the Coston2 RPC endpoint, and your Gemini API key.
2. Set `LOCAL_MODE=false` (required on live networks, enables
   attestation).
3. Start an ngrok tunnel to the proxy's external port and set
   `EXT_PROXY_URL` to the generated HTTPS URL.
4. Run `./scripts/pre-build.sh` to deploy `InstructionSender` and
   register the extension on the `TeeExtensionRegistry`.
5. Start services with:
   ```bash
   docker compose -f docker-compose.yaml -f docker-compose.coston2.yaml up -d --build
   ```
6. Run `./scripts/post-build.sh` to register the TEE version and TEE
   machine on-chain.
7. Run `./scripts/test.sh` to send a test incident report through the
   extension and confirm the classification and hash come back correctly.

Local development against a Hardhat node follows the same flow with
`LOCAL_MODE=true` and no ngrok tunnel required; see the scaffold's
`docs/` folder for the full walkthrough if you're setting up a fresh
environment.

## Why this matters for the submission

The core privacy claim of Safety Incident Log is that raw report text
never touches a public blockchain. Layer 1 handles that at the storage
level (only a hash, severity, and timestamp go on-chain). This extension
closes the remaining gap: the *processing* of that raw text, the AI call
itself, also happens somewhere the operator can't inspect it, rather than
on a conventional server.