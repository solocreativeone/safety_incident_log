"""
One-off script: log a single synthetic, non-sensitive test incident on
BOT Chain (testnet or mainnet) to verify the IncidentLog contract works
end-to-end. Updated for the reporterId-enabled contract.

Usage:
    python3 test_botchain_incident.py <contract_address> --network testnet
    python3 test_botchain_incident.py <contract_address> --network mainnet
"""

import argparse
import json
import os

from web3 import Web3
from web3.middleware import ExtraDataToPOAMiddleware

NETWORKS = {
    "testnet": {
        "rpc_url": "https://rpc.bohr.life",
        "chain_id": 968,
        "explorer": "https://scan.bohr.life",
    },
    "mainnet": {
        "rpc_url": "https://rpc.botchain.ai",
        "chain_id": 677,
        "explorer": "https://scan.botchain.ai",
    },
}

INCIDENT_LOG_ABI = json.loads("""
[
  {"inputs":[{"internalType":"bytes32","name":"recordHash","type":"bytes32"},{"internalType":"bytes32","name":"reporterId","type":"bytes32"},{"internalType":"uint8","name":"severity","type":"uint8"}],"name":"logIncident","outputs":[{"internalType":"uint256","name":"id","type":"uint256"}],"stateMutability":"nonpayable","type":"function"}
]
""")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("contract_address")
    parser.add_argument("--network", required=True, choices=NETWORKS.keys())
    args = parser.parse_args()

    net = NETWORKS[args.network]
    private_key = os.environ["BOTCHAIN_PRIVATE_KEY"]

    print(f"Using network: {args.network} (chainId={net['chain_id']}, rpc={net['rpc_url']})")

    w3 = Web3(Web3.HTTPProvider(net["rpc_url"]))
    w3.middleware_onion.inject(ExtraDataToPOAMiddleware, layer=0)

    if not w3.is_connected():
        raise ConnectionError(f"Could not connect to {net['rpc_url']}")

    actual_chain_id = w3.eth.chain_id
    if actual_chain_id != net["chain_id"]:
        raise RuntimeError(
            f"RPC chain ID mismatch: expected {net['chain_id']} for {args.network}, "
            f"but {net['rpc_url']} reports {actual_chain_id}. Refusing to proceed."
        )

    account = w3.eth.account.from_key(private_key)
    contract = w3.eth.contract(
        address=Web3.to_checksum_address(args.contract_address), abi=INCIDENT_LOG_ABI
    )

    test_payload = json.dumps({
        "summary": "Synthetic test incident for BOT Chain verification",
        "severity": "MEDIUM",
        "driving_factor": "test",
        "justification": "This is a placeholder record for functional testing only.",
        "network": args.network,
    }, sort_keys=True).encode("utf-8")
    record_hash = w3.keccak(test_payload)
    reporter_id = w3.keccak(text="test-script-synthetic-reporter")
    severity = 1  # MEDIUM

    tx = contract.functions.logIncident(record_hash, reporter_id, severity).build_transaction({
        "from": account.address,
        "nonce": w3.eth.get_transaction_count(account.address),
        "chainId": net["chain_id"],
    })

    signed = w3.eth.account.sign_transaction(tx, private_key=private_key)
    raw_tx = getattr(signed, "rawTransaction", getattr(signed, "raw_transaction", None))
    tx_hash = w3.eth.send_raw_transaction(raw_tx)
    receipt = w3.eth.wait_for_transaction_receipt(tx_hash)

    if receipt.status != 1:
        raise RuntimeError(
            f"Transaction reverted. Check that {args.contract_address} is the "
            f"correct, currently deployed IncidentLog contract for {args.network}."
        )

    tx_hash_hex = tx_hash.hex()
    if not tx_hash_hex.startswith("0x"):
        tx_hash_hex = "0x" + tx_hash_hex

    print(f"Success! Block: {receipt.blockNumber}")
    print(f"Transaction: {net['explorer']}/tx/{tx_hash_hex}")


if __name__ == "__main__":
    main()