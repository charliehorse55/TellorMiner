package cli

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jawher/mow.cli"
	tellorCommon "github.com/tellor-io/TellorMiner/common"
	"github.com/tellor-io/TellorMiner/config"
	"github.com/tellor-io/TellorMiner/contracts"
	"github.com/tellor-io/TellorMiner/contracts1"
	"github.com/tellor-io/TellorMiner/db"
	"github.com/tellor-io/TellorMiner/ops"
	"github.com/tellor-io/TellorMiner/rpc"
	"github.com/tellor-io/TellorMiner/util"
	"log"
	"math/big"
	"os"
)

var ctx context.Context

func ErrorHandler(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		cli.Exit(-1)
	}
}

func ErrorWrapper(f func() error) func() {
	return func() {
		ErrorHandler(f())
	}
}

func buildContext() error {
	cfg := config.GetConfig()
	//create an rpc client
	client, err := rpc.NewClient(cfg.NodeURL)
	if err != nil {
		log.Fatal(err)
	}
	//create an instance of the tellor master contract for on-chain interactions
	contractAddress := common.HexToAddress(cfg.ContractAddress)
	masterInstance, err := contracts.NewTellorMaster(contractAddress, client)
	transactorInstance, err := contracts1.NewTellorTransactor(contractAddress, client)
	if err != nil {
		log.Fatal(err)
	}

	ctx = context.WithValue(context.Background(), tellorCommon.ClientContextKey, client)
	ctx = context.WithValue(ctx, tellorCommon.ContractAddress, contractAddress)
	ctx = context.WithValue(ctx, tellorCommon.MasterContractContextKey, masterInstance)
	ctx = context.WithValue(ctx, tellorCommon.TransactorContractContextKey, transactorInstance)

	privateKey, err := crypto.HexToECDSA(cfg.PrivateKey)
	if err != nil {
		return fmt.Errorf("problem getting private key: %s", err.Error())
	}
	ctx = context.WithValue(ctx, tellorCommon.PrivateKey, privateKey)

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("error casting public key to ECDSA")
	}

	publicAddress := crypto.PubkeyToAddress(*publicKeyECDSA)
	ctx = context.WithValue(ctx, tellorCommon.PublicAddress, publicAddress)

	//Issue #55, halt if client is still syncing with Ethereum network
	s, err := client.IsSyncing(ctx)
	if err != nil {
		return fmt.Errorf("could not determine if Ethereum client is syncing: %v\n", err)
	}
	if s {
		return fmt.Errorf("ethereum node is still sycning with the network")
	}
	return nil
}

func AddDBToCtx() error {
	cfg := config.GetConfig()
	//create a db instance
	os.RemoveAll(cfg.DBFile)
	DB, err := db.Open(cfg.DBFile)
	if err != nil {
		return err
	}

	var dataProxy db.DataServerProxy
	if true { //EVANTODO
		proxy, err := db.OpenLocalProxy(DB)
		if err != nil {
			return err
		}
		dataProxy = proxy
	} else {
		proxy, err := db.OpenRemoteDB(DB)
		if err != nil {
			return err
		}
		dataProxy = proxy
	}
	ctx = context.WithValue(ctx, tellorCommon.DataProxyKey, dataProxy)
	ctx = context.WithValue(ctx, tellorCommon.DBContextKey, DB)
	return nil
}


func App() *cli.Cli {


	app := cli.App("TellorMiner", "The tellor.io official miner")

	//app wide config options
	configPath := app.StringOpt("config", "config.json", "Path to the primary JSON config file")
	logPath := app.StringOpt("logConfig", "loggingConfig.json", "Path to a JSON logging config file")

	//this will get run before any of the commands
	app.Before = func() {
		ErrorHandler(config.ParseConfig(*configPath))
		ErrorHandler(util.ParseLoggingConfig(*logPath))
		ErrorHandler(buildContext())
	}

	app.Command("stake", "staking operations", stakeCmd)
	app.Command("transfer", "send TRB to address", moveCmd(ops.Transfer))
	app.Command("approve", "approve TRB to address", moveCmd(ops.Approve))
	app.Command("balance", "check balance of address", balanceCmd)
	app.Command("dispute", "dispute operations", disputeCmd)
	app.Command("activity", "block activity", activityCmd)
	return app
}

func stakeCmd(cmd *cli.Cmd) {
	cmd.Command("deposit", "deposit TRB stake", simpleCmd(ops.Deposit))
	cmd.Command("withdraw", "withdraw TRB stake", simpleCmd(ops.WithdrawStake))
	cmd.Command("request", "request to withdraw TRB stake", simpleCmd(ops.RequestStakingWithdraw))
	cmd.Command("status", "show current staking status", simpleCmd(ops.ShowStatus))
}

func simpleCmd(f func (context.Context) error) func(*cli.Cmd) {
	return func(cmd *cli.Cmd) {
		cmd.Action = func() {
			ErrorHandler(f(ctx))
		}
	}
}

func moveCmd(f func(common.Address, *big.Int, context.Context) error) func (*cli.Cmd) {
	return func(cmd *cli.Cmd) {
		amt := TRBAmount{}
		addr := ETHAddress{}
		cmd.VarArg("AMOUNT", &amt, "amount to transfer")
		cmd.VarArg("ADDRESS", &addr, "ethereum public address")
		cmd.Action = func() {
			ErrorHandler(f(addr.addr, amt.Int, ctx))
		}
	}
}

func balanceCmd(cmd *cli.Cmd) {
	addr := ETHAddress{}
	cmd.VarArg("ADDRESS", &addr, "ethereum public address")
	cmd.Spec = "[ADDRESS]"
	cmd.Action = func() {
		var zero [20]byte
		if bytes.Compare(addr.addr.Bytes(), zero[:]) == 0 {
			addr.addr = ctx.Value(tellorCommon.PublicAddress).(common.Address)
		}
		ErrorHandler(ops.Balance(ctx, addr.addr))
	}
}

func disputeCmd(cmd *cli.Cmd) {
	cmd.Command("vote", "deposit TRB stake", voteCmd)
	cmd.Command("new", "withdraw TRB stake", newDisputeCmd)
}

func voteCmd(cmd *cli.Cmd) {
	disputeID := EthereumInt{}
	cmd.VarArg("DISPUTE_ID", &disputeID, "dispute id")
	supports := cmd.BoolArg("SUPPORT", false, "do you support the dispute? (true|false)")
	cmd.Action = func() {
		ErrorHandler(ops.Vote(disputeID.Int, *supports, ctx))
	}
}

func newDisputeCmd(cmd *cli.Cmd) {
	requestID := EthereumInt{}
	timestamp := EthereumInt{}
	minerIndex := EthereumInt{}
	cmd.VarArg("REQUEST_ID", &requestID, "request id")
	cmd.VarArg("TIMESTAMP", &timestamp, "timestamp")
	cmd.VarArg("MINER_INDEX", &minerIndex, "miner to dispute (0-4)")
	cmd.Action = func() {
		ErrorHandler(ops.Dispute(requestID.Int, timestamp.Int, minerIndex.Int, ctx))
	}
}

func activityCmd(cmd *cli.Cmd) {
	cmd.Before = ErrorWrapper(AddDBToCtx)
	simpleCmd(ops.ActivityFoo)(cmd)
}