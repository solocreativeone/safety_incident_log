const { expect } = require("chai");
const { ethers } = require("hardhat");

describe("IncidentLog", function () {
  let incidentLog;
  let owner;

  const Severity = { Low: 0, Medium: 1, High: 2, Critical: 3 };

  beforeEach(async function () {
    [owner] = await ethers.getSigners();
    const IncidentLog = await ethers.getContractFactory("IncidentLog");
    incidentLog = await IncidentLog.deploy();
    await incidentLog.waitForDeployment();
  });

  it("starts with zero incidents", async function () {
    expect(await incidentLog.incidentCount()).to.equal(0);
  });

  it("logs a Medium severity incident and emits an event", async function () {
    const recordHash = ethers.keccak256(ethers.toUtf8Bytes("incident-report-1"));

    await expect(incidentLog.logIncident(recordHash, Severity.Medium))
      .to.emit(incidentLog, "IncidentLogged")
      .withArgs(0, recordHash, Severity.Medium, anyValue(), owner.address);

    expect(await incidentLog.incidentCount()).to.equal(1);
  });

  it("stores retrievable incident data", async function () {
    const recordHash = ethers.keccak256(ethers.toUtf8Bytes("incident-report-2"));
    await incidentLog.logIncident(recordHash, Severity.Critical);

    const [storedHash, severity, timestamp, reporter] = await incidentLog.getIncident(0);
    expect(storedHash).to.equal(recordHash);
    expect(severity).to.equal(Severity.Critical);
    expect(reporter).to.equal(owner.address);
    expect(timestamp).to.be.greaterThan(0);
  });

  it("increments ids across multiple incidents", async function () {
    const hash1 = ethers.keccak256(ethers.toUtf8Bytes("a"));
    const hash2 = ethers.keccak256(ethers.toUtf8Bytes("b"));

    await incidentLog.logIncident(hash1, Severity.Low);
    await incidentLog.logIncident(hash2, Severity.High);

    expect(await incidentLog.incidentCount()).to.equal(2);
  });
});

// Small helper since chai-matchers' anyValue isn't imported by default in all setups
function anyValue() {
  return (x) => true;
}
