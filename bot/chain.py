import json
import os

from web3 import Web3
from web3.middleware import geth_poa_middleware

from triage import Severity, TriageResult

# One place defining every chain this bot can write to. Adding a new
# chain later means adding one entry here — never a hardcoded URL
# scattered somewhere else in the file.
NETWORKS = {
    "coston2": {
        "rpc_url": os.environ.get("COSTON2_RPC_URL", "https://coston2-api.flare.network/ext/C/rpc"),
        "chain_id": 114,
        "contract_address": os.environ.get("COSTON2_INCIDENT_LOG_ADDRESS"),
        "private_key": os.environ.get("PRIVATE_KEY"),
        "explorer": "https://coston2-explorer.flare.network",
        "poa": False,
    },
    "botchain_testnet": {
        "rpc_url": "https://rpc.bohr.life",
        "chain_id": 968,
        "contract_address": os.environ.get("BOTCHAIN_TESTNET_INCIDENT_LOG_ADDRESS"),
        "private_key": os.environ.get("BOTCHAIN_PRIVATE_KEY"),
        "explorer": "https://scan.bohr.life",
        "poa": True,
    },
    "botchain_mainnet": {
        "rpc_url": "https://rpc.botchain.ai",
        "chain_id": 677,
        "contract_address": os.environ.get("BOTCHAIN_MAINNET_INCIDENT_LOG_ADDRESS"),
        "private_key": os.environ.get("BOTCHAIN_PRIVATE_KEY"),
        "explorer": "https://scan.botchain.ai",
        "poa": True,
    },
}

INCIDENT_LOG_ABI = json.loads("""
[
  {
    "inputs": [
      {"internalType": "bytes32", "name": "recordHash", "type": "bytes32"},
      {"internalType": "bytes32", "name": "reporterId", "type": "bytes32"},
      {"internalType": "uint8", "name": "severity", "type": "uint8"}
    ],
    "name": "logIncident",
    "outputs": [{"internalType": "uint256", "name": "id", "type": "uint256"}],
    "stateMutability": "nonpayable",
    "type": "function"
  },
  {
    "inputs": [],
    "name": "incidentCount",
    "outputs": [{"internalType": "uint256", "name": "", "type": "uint256"}],
    "stateMutability": "view",
    "type": "function"
  }
]
""")


def _client(network: str) -> Web3:
    net = NETWORKS[network]
    w3 = Web3(Web3.HTTPProvider(net["rpc_url"]))
    if net["poa"]:
        w3.middleware_onion.inject(geth_poa_middleware, layer=0)

    if not w3.is_connected():
        raise ConnectionError(f"Could not connect to {network} RPC at {net['rpc_url']}")

    # Refuse to proceed on a chain-ID mismatch — this is what caught the
    # earlier testnet/mainnet mixup and prevents it happening silently again.
    actual_chain_id = w3.eth.chain_id
    if actual_chain_id != net["chain_id"]:
        raise RuntimeError(
            f"Chain ID mismatch for {network}: expected {net['chain_id']}, "
            f"RPC reports {actual_chain_id}. Refusing to proceed."
        )
    return w3


def record_hash(result: TriageResult) -> bytes:
    w3 = Web3()
    payload = json.dumps(result.record_payload(), sort_keys=True).encode("utf-8")
    return w3.keccak(payload)


def reporter_id_hash(telegram_user_id: str | int) -> bytes:
    w3 = Web3()
    return w3.keccak(text=str(telegram_user_id))


def log_incident_on_chain(result: TriageResult, telegram_user_id: str | int, network: str = "coston2") -> dict:
    if not result.should_log_on_chain():
        raise ValueError(f"Severity {result.severity.name} is below the on-chain threshold.")

    net = NETWORKS[network]
    if not net["contract_address"]:
        raise ValueError(f"No contract address configured for network '{network}'.")

    w3 = _client(network)
    account = w3.eth.account.from_key(net["private_key"])
    contract = w3.eth.contract(address=Web3.to_checksum_address(net["contract_address"]), abi=INCIDENT_LOG_ABI)

    tx = contract.functions.logIncident(
        record_hash(result),
        reporter_id_hash(telegram_user_id),
        int(result.severity),
    ).build_transaction({
        "from": account.address,
        "nonce": w3.eth.get_transaction_count(account.address),
        "chainId": net["chain_id"],
    })

    signed = w3.eth.account.sign_transaction(tx, private_key=net["private_key"])
    raw_tx = getattr(signed, "rawTransaction", getattr(signed, "raw_transaction", None))
    tx_hash = w3.eth.send_raw_transaction(raw_tx)
    receipt = w3.eth.wait_for_transaction_receipt(tx_hash)

    tx_hash_hex = tx_hash.hex()
    if not tx_hash_hex.startswith("0x"):
        tx_hash_hex = "0x" + tx_hash_hex

    return {
        "tx_hash": tx_hash_hex,
        "block_number": receipt.blockNumber,
        "explorer_url": f"{net['explorer']}/tx/{tx_hash_hex}",
    }