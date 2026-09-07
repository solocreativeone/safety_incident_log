# Safety Incident Log: Flare Summer Signal (Bounty 2: Confidential Compute Apps)

AI-triaged, verifiable workplace safety-incident reporting. Workers report
incidents via Telegram in plain language; an always-on AI layer classifies
severity against a 4-tier rubric (Low/Medium/High/Critical, weighing injury
risk, equipment damage, and near-miss/systemic signals); Medium+ incidents
are logged on Coston2 as a tamper-evident record (hash, severity tier,
reporterId, and timestamp only, no raw report text on-chain); the safety
officer is alerted and makes the final response call.

## Status

- [x] **Layer 1: Coston2 contract** (`contracts/IncidentLog.sol`): written,
      tested, and deployed to Coston2.
      Original submitted contract (judged deployment):
      `0x9cc807c3c1FaFc9B5cF3673C08004293721cA161`.
      Updated contract with reporterId (post-submission, see below):
      `0x49dA1A09D36bF8F525D20c560154a7aFCDeB77b3`.
- [x] **Python triage + bot** (`bot/`): severity classification, on-chain
      hook, and Telegram entrypoint all tested end to end against a live
      bot token and working.
- [x] **Layer 2: Flare Compute Extension** (confidential compute): completed.
      See [`fce-extension-scaffold/`](./fce-extension-scaffold/) for the
      TEE extension code and setup instructions.
- [x] **BOT Chain deployment (exploratory)**: the same contract also
      deployed to BOT Chain testnet and mainnet, unrelated to the Flare
      submission. See [`botchain/`](./botchain/) for details.

## Project structure

```text
├── contracts/
│   └── IncidentLog.sol         # Solidity contract (Coston2 target)
├── test/
│   └── IncidentLog.test.js     # Hardhat contract tests
├── scripts/
│   └── deploy.js               # Deployment script across networks
├── bot/
│   ├── triage.py               # AI severity classification
│   ├── chain.py                # web3.py bridge (triage -> logIncident)
│   ├── main.py                 # Telegram bot entrypoint
│   └── requirements.txt        # Python dependencies
├── botchain/
│   └── test_botchain_incident.py # Script for testing on BOT Chain
├── hardhat.config.js           # Network config (Coston2, BOT Chain)
└── README.md
```

## Setup

1. `npm install` installs Hardhat and the toolbox.
2. `pip install -r bot/requirements.txt --break-system-packages` (or in a venv).
3. Copy `.env.example` to `.env` and fill in:
   - A **testnet-only** private key, funded with C2FLR from the Coston2 faucet.
   - Your Telegram bot token (from @BotFather) and the safety officer's chat ID.
   - Your Gemini API key.
4. `npx hardhat test` compiles and runs the contract tests.
   > Note: compiling requires downloading the solc compiler from
   > `binaries.soliditylang.org`. If you are on a restricted network (e.g. a
   > sandboxed dev environment), this step needs to run somewhere with open
   > internet access.
5. `npx hardhat run scripts/deploy.js --network coston2` deploys `IncidentLog`
   to Coston2 and prints the deployed address. Paste it into `.env` as
   `COSTON2_INCIDENT_LOG_ADDRESS`.
6. `python3 bot/main.py` starts the Telegram bot.

You can also smoke-test the triage rubric alone, without Telegram or the
chain, by running: 

`cd bot && python triage.py`


This runs a few sample incident reports through the classifier and prints
the assigned severity, driving factor, and reasoning, which is useful for
tuning the rubric prompt before wiring up the full bot.

## Design notes

- **Why always-on AI classification, not human-flagged**: a worker under
  stress or using casual language may not correctly self-flag urgency.
  The AI layer exists specifically to catch that gap, not just format
  what a human already triaged.

- **Human-in-the-loop stays downstream**: the AI classifies and alerts;
  the safety officer decides and acts. This split is deliberate: AI for
  constant vigilance at machine scale, human for judgment and accountability.

- **On-chain footprint is minimal by design**: only a record hash,
  reporterId, severity tier, and timestamp are ever written to
  `IncidentLog.sol`. The raw report (names, injury specifics, exact
  location) stays off-chain, which is also what sets up Layer 2: running
  the triage step itself inside a Flare Compute Extension so even the
  *processing* of that raw text happens in a hardware-isolated
  environment, not just its storage.

## Layer 2: Confidential Compute

Completed. The severity-classification logic has been ported to run
inside the TEE using the Flare Compute Extension.

## Post-submission update (v2): reporterId

After the Flare Summer Signal submission, the Flare team gave feedback on
worker attribution: the original design had every incident attributed
only to the relayer bot, with no way to trace which specific worker
reported it. Per their suggestion, a `reporterId` field was added.

`reporterId` is a keccak256 hash of the worker's Telegram user ID,
computed off-chain and passed into `logIncident` alongside the existing
`recordHash` and `severity` fields. This gives each incident a
consistent, pseudonymous, on-chain identity for the reporting worker,
without custodying a separate wallet or private key per worker. The
single-relayer signing model stays the same, avoiding the larger key
management risk that a per-user wallet approach would introduce.

This is a v2 improvement, made after the hackathon deadline, and is
separate from the original judged submission.

Updated `logIncident` signature:

```solidity
function logIncident(bytes32 recordHash, bytes32 reporterId, Severity severity) external returns (uint256 id)
```
