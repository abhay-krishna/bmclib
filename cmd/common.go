package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bmc-toolbox/bmcbutler/pkg/asset"
	"github.com/bmc-toolbox/bmcbutler/pkg/butler"

	"github.com/bmc-toolbox/bmcbutler/pkg/inventory"
	"github.com/bmc-toolbox/bmcbutler/pkg/metrics"
)

var (
	exitFlag      bool
	butlerManager butler.ButlerManager
	metricsChan   chan []metrics.MetricsMsg
	commandWG     sync.WaitGroup
)

// post handles clean up actions
// - closes the butler channel
// - Waits for all go routines in commandWG to finish.
func post(butlerChan chan butler.ButlerMsg) {
	close(butlerChan)
	commandWG.Wait()
}

// pre sets up required plumbing and returns two channels.
// - Spawn go routine to listen to interrupt signals
// - Setup metrics channel
// - Spawn the metrics forwarder go routine
// - Setup the inventory channel over which to recieve assets
// - Based on the inventory source (dora/csv), Spawn the asset retriever go routine.
// - Spawn butlers
// - Return inventory channel, butler channel.
func pre() (inventoryChan chan []asset.Asset, butlerChan chan butler.ButlerMsg) {

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		_ = <-sigChan
		exitFlag = true
	}()

	// A channel butlers sends metrics to the metrics sender
	metricsChan = make(chan []metrics.MetricsMsg, 5)

	//the metrics forwarder routine
	metricsForwarder := metrics.Metrics{
		Config:  runConfig,
		Logger:  log,
		Channel: metricsChan,
		SyncWG:  &commandWG,
	}

	//metrics emitter instance, used by methods to emit metrics to the forwarder.
	metricsEmitter := metrics.Emitter{Channel: metricsChan}

	//spawn metrics forwarder routine
	go metricsForwarder.Run()
	commandWG.Add(1)

	// A channel to recieve inventory assets
	inventoryChan = make(chan []asset.Asset, 5)

	//determine inventory to fetch asset data.
	inventorySource := runConfig.InventoryParams.Source

	//if --ip was passed, set inventorySource
	if runConfig.FilterParams.Ip != "" {
		inventorySource = "iplist"
	}

	//based on inventory source, invoke assetRetriever
	var assetRetriever func()

	switch inventorySource {
	case "csv":
		inventoryInstance := inventory.Csv{
			Config:     runConfig,
			Log:        log,
			AssetsChan: inventoryChan,
		}

		assetRetriever = inventoryInstance.AssetRetrieve()
	case "dora":
		inventoryInstance := inventory.Dora{
			Config:         runConfig,
			Log:            log,
			BatchSize:      10,
			AssetsChan:     inventoryChan,
			MetricsEmitter: metricsEmitter,
		}

		assetRetriever = inventoryInstance.AssetRetrieve()
	case "iplist":
		inventoryInstance := inventory.IpList{
			Channel:   inventoryChan,
			Config:    runConfig,
			BatchSize: 1,
			Log:       log,
		}

		assetRetriever = inventoryInstance.AssetRetrieve()
	default:
		fmt.Println("Unknown/no inventory source declared in cfg: ", inventorySource)
		os.Exit(1)
	}

	go assetRetriever()
	// Spawn butlers to work
	butlerChan = make(chan butler.ButlerMsg, 5)
	butlerManager = butler.ButlerManager{
		ButlerChan:     butlerChan,
		Config:         runConfig,
		Log:            log,
		MetricsEmitter: metricsEmitter,
		SyncWG:         &commandWG,
	}

	go butlerManager.SpawnButlers()
	commandWG.Add(1)

	//give the butlers a second to spawn.
	time.Sleep(1 * time.Second)

	return inventoryChan, butlerChan
}
