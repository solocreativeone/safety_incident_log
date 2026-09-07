// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/// @title IncidentLog
/// @notice Tamper-evident log of workplace safety incidents.
/// Only a hash of the full incident record is stored on-chain — the raw
/// report text never touches this contract. reporterId is a pseudonymous
/// hash of the worker's identity (e.g. keccak256(telegramUserId)), not a
/// wallet — attribution without custodying a per-worker key.
contract IncidentLog {
    enum Severity { Low, Medium, High, Critical }

    struct Incident {
        bytes32 recordHash;
        bytes32 reporterId;
        Severity severity;
        uint256 timestamp;
        address reporter; // the relayer/bot's signing address, not the worker
    }

    Incident[] private incidents;

    event IncidentLogged(
        uint256 indexed id,
        bytes32 recordHash,
        bytes32 indexed reporterId,
        Severity severity,
        uint256 timestamp,
        address indexed reporter
    );

    function logIncident(
        bytes32 recordHash,
        bytes32 reporterId,
        Severity severity
    ) external returns (uint256 id) {
        id = incidents.length;
        incidents.push(Incident({
            recordHash: recordHash,
            reporterId: reporterId,
            severity: severity,
            timestamp: block.timestamp,
            reporter: msg.sender
        }));
        emit IncidentLogged(id, recordHash, reporterId, severity, block.timestamp, msg.sender);
    }

    function incidentCount() external view returns (uint256) {
        return incidents.length;
    }

    function getIncident(uint256 id) external view returns (
        bytes32 recordHash,
        bytes32 reporterId,
        Severity severity,
        uint256 timestamp,
        address reporter
    ) {
        Incident storage inc = incidents[id];
        return (inc.recordHash, inc.reporterId, inc.severity, inc.timestamp, inc.reporter);
    }
}