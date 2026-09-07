package main

import (
	"context"
	"flag"
	"fmt"
	"math/big"

	"extension-scaffold/tools/pkg/configs"
	"extension-scaffold/tools/pkg/fccutils"
	"extension-scaffold/tools/pkg/support"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/flare-foundation/go-flare-common/pkg/contracts/tee/machinemanager"
	"github.com/flare-foundation/go-flare-common/pkg/logger"
)

func main() {
	af := flag.String("a", configs.AddressesFile, "file with deployed addresses")
	cf := flag.String("c", "", "chain node url")
	rpcF := flag.String("rpc", "", "chain node json-rpc url")
	rpcUrlF := flag.String("rpc-url", "", "chain node json-rpc url")
	pf := flag.String("p", configs.ExtensionProxyURL, "extension proxy url (used to query TEE info)")
	extIDFlag := flag.Int64("ext", 66135, "extension ID to clean up stale machines for")
	teeIDFlag := flag.String("teeId", "", "specific TEE ID to pause (0x...)")
	pauseAllStale := flag.Bool("all-stale", false, "pause all active machines for extension that do not match current node")

	flag.Parse()

	chainNodeURL := fccutils.GetRPCURL(*rpcF, *rpcUrlF, *cf)
	s, err := support.DefaultSupport(*af, chainNodeURL)
	if err != nil {
		fccutils.FatalWithCause(err)
	}

	callOpts := &bind.CallOpts{Context: context.Background()}

	// If a specific teeId is requested, pause it directly.
	if *teeIDFlag != "" {
		targetTee := common.HexToAddress(*teeIDFlag)
		pauseMachine(s, targetTee)
		return
	}

	// Query proxy for current node's TeeID
	info, err := fccutils.TeeInfo(*pf)
	if err != nil {
		logger.Warnf("Could not query proxy /info at %s: %v", *pf, err)
	}
	var currentNodeTeeID common.Address
	if info != nil {
		currentNodeTeeID, _, err = fccutils.TeeProxyId(info)
		if err == nil {
			logger.Infof("Current TEE node TeeID (from proxy %s): %s", *pf, currentNodeTeeID.Hex())
		}
	}

	extID := big.NewInt(*extIDFlag)
	logger.Infof("Querying active machines for extension %s on-chain...", extID.String())
	activeRes, err := s.TeeMachineRegistry.GetActiveTeeMachines(callOpts, extID)
	if err != nil {
		fccutils.FatalWithCause(fmt.Errorf("failed to query active TEE machines: %w", err))
	}

	if len(activeRes.TeeIds) == 0 {
		logger.Infof("No active machines found for extension %s.", extID.String())
		return
	}

	logger.Infof("Found %d active machine(s) for extension %s:", len(activeRes.TeeIds), extID.String())
	for i, id := range activeRes.TeeIds {
		isCurrent := (id == currentNodeTeeID)
		st, _ := s.TeeMachineRegistry.GetTeeMachineStatus(callOpts, id)
		owner, _ := s.TeeMachineRegistry.GetTeeMachineOwner(callOpts, id)
		logger.Infof("  [%d] %s (status=%d, owner=%s, current=%v) url=%q", i, id.Hex(), st, owner.Hex(), isCurrent, activeRes.Urls[i])
	}

	if *pauseAllStale {
		for _, id := range activeRes.TeeIds {
			if id == currentNodeTeeID && currentNodeTeeID != (common.Address{}) {
				logger.Infof("Skipping current node machine %s", id.Hex())
				continue
			}
			logger.Infof("Pausing stale machine %s...", id.Hex())
			pauseMachine(s, id)
		}
	} else {
		logger.Infof("To pause stale machines, run with -all-stale flag or specify -teeId 0x...")
	}
}

func pauseMachine(s *support.Support, teeID common.Address) {
	opts, err := bind.NewKeyedTransactorWithChainID(s.Prv, s.ChainID)
	if err != nil {
		fccutils.FatalWithCause(err)
	}

	tx, err := s.TeeMachineRegistry.Pause(opts, teeID)
	if err != nil {
		reason := fccutils.DecodeRevertReason(err)
		if mmABI, abiErr := machinemanager.MachineManagerMetaData.GetAbi(); abiErr == nil {
			if callData, packErr := mmABI.Pack("pause", teeID); packErr == nil {
				from := crypto.PubkeyToAddress(s.Prv.PublicKey)
				raw := fccutils.SimulateAndDecodeRevert(s.ChainClient, from, s.Addresses.FlareTeeManager, nil, callData)
				if named := fccutils.MatchCustomError(mmABI, raw); named != "" {
					reason = named
				} else if reason == "" {
					reason = raw
				}
			}
		}
		if reason == "" {
			reason = err.Error()
		}
		fccutils.FatalWithCause(fmt.Errorf("pause transaction failed for TEE %s: %s", teeID.Hex(), reason))
	}

	logger.Infof("Pause tx submitted: %s (waiting for mining...)", tx.Hash().Hex())
	receipt, err := support.CheckTx(tx, s.ChainClient)
	if err != nil {
		fccutils.FatalWithCause(fmt.Errorf("pause transaction receipt error: %w", err))
	}

	logger.Infof("Successfully paused TEE machine %s (status=%d in tx %s)", teeID.Hex(), receipt.Status, receipt.TxHash.Hex())
}
