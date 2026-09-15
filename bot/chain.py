from triage import TriageResult
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

    if receipt.status != 1:
        raise RuntimeError(
            f"Transaction reverted or failed on network '{network}'. "
            f"Check that contract_address ({net['contract_address']}) is the "
            f"deployed IncidentLog contract, not a plain wallet address, and "
            f"that its ABI matches the version being called."
        )

    tx_hash_hex = tx_hash.hex()
    if not tx_hash_hex.startswith("0x"):
        tx_hash_hex = "0x" + tx_hash_hex

    return {
        "tx_hash": tx_hash_hex,
        "block_number": receipt.blockNumber,
        "explorer_url": f"{net['explorer']}/tx/{tx_hash_hex}",
    }
