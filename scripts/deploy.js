const hre = require("hardhat");

const EXPLORERS = {
  coston2: "https://coston2-explorer.flare.network",
  botchainTestnet: "https://scan.bohr.life",
};

async function main() {
  const IncidentLog = await hre.ethers.getContractFactory("IncidentLog");
  const incidentLog = await IncidentLog.deploy();
  await incidentLog.waitForDeployment();

  const address = await incidentLog.getAddress();
  const explorerBase = EXPLORERS[hre.network.name];

  console.log(`IncidentLog deployed to: ${address}`);
  console.log(`Network: ${hre.network.name}`);
  if (explorerBase) {
    console.log(`Verify on explorer: ${explorerBase}/address/${address}`);
  }
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});