package ticketcache

import (
	"errors"
	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/crypto"
	"github.com/PlatONnetwork/PlatON-Go/ethdb"
	"github.com/PlatONnetwork/PlatON-Go/log"
	"github.com/PlatONnetwork/PlatON-Go/p2p/discover"
	"github.com/golang/protobuf/proto"
	"math/big"
	"sort"
	"sync"
	"time"
)

type TicketCache map[discover.NodeID][]common.Hash

type Timer struct {
	start time.Time
}

func (t *Timer) Begin() {
	t.start = time.Now()
	//fmt.Println("Begin=> ", "now: ", time.Now().Nanosecond(), " tCalc: ", t.tCalc)
}

func (t *Timer) End() float64 {
	//fmt.Println("End=> ", "now: ", time.Now().Nanosecond(), " tCalc: ", t.tCalc)
	tns := time.Since(t.start).Nanoseconds()
	tms := float64(tns) / float64(1e6)
	return tms

}

var (
	//error def
	ErrNotfindFromNodeId = errors.New("Not find tickets from node id")
	ErrProbufMarshal     = errors.New("protocol buffer Marshal faile")
	ErrLeveldbPut        = errors.New("level db put faile")

	//const def
	ticketPoolCacheKey = []byte("ticketPoolCache")
)

//var ticketidsCache *NumBlocks

<<<<<<< HEAD
type TicketTempCache struct{
	Cache 		*NumBlocks
	lock 		*sync.Mutex
=======
type TicketTempCache struct {
	Cache *NumBlocks
	lock  *sync.Mutex
>>>>>>> localdev/develop
}

// global obj of ticket related
var ticketTemp *TicketTempCache

//func NewTicketIdsCache(db ethdb.Database) *NumBlocks {
func NewTicketIdsCache(db ethdb.Database) *TicketTempCache {
	/*
		append: New votes for ticket purchases
		Del: Node elimination，ticket expired，ticket release
	*/
	//logInfo("NewTicketIdsCache==> Init ticketidsCache call NewTicketIdsCache func")
	timer := Timer{}
	timer.Begin()
	if nil != ticketTemp {
		return ticketTemp
	}
	ticketTemp = &TicketTempCache{
		Cache: &NumBlocks{
			NBlocks: make(map[string]*BlockNodes),
		},

		lock: &sync.Mutex{},
	}

	if cache, err := db.Get(ticketPoolCacheKey); nil != err {
<<<<<<< HEAD
		log.Warn("Failed call ticketcache NewTicketIdsCache to get Global Cache by levelDB", "err", err)
	}else {
=======
		log.Warn("Warn call ticketcache NewTicketIdsCache to get Global Cache by levelDB", "err", err)
	} else {
>>>>>>> localdev/develop
		log.Info("Call ticketcache NewTicketIdsCache to Unmarshal Global Cache", "Cachelen: ", len(cache))
		//if err := proto.Unmarshal(cache, ticketidsCache); err != nil {
		if err := proto.Unmarshal(cache, ticketTemp.Cache); err != nil {
			log.Error("Failed call NewTicketIdsCache to Unmarshal Global Cache", "err", err)
		}
	}
	log.Info("Call ticketcache NewTicketIdsCache finish ...", "ms: ", timer.End())
	return ticketTemp
}

// Create a ticket cache by blocknumber and blockHash from global temp
func GetNodeTicketsCacheMap(blocknumber *big.Int, blockhash common.Hash) (ret TicketCache) {
	log.Info("Call ticketcache GetNodeTicketsCacheMap", "blocknumber: ", blocknumber, " blockhash: ", blockhash.Hex())
	if ticketTemp != nil {

		// getting a ticket cache by blocknumber and blockHash from global temp
		ret = ticketTemp.GetNodeTicketsMap(blocknumber, blockhash)
	} else {
<<<<<<< HEAD
		log.Warn("Failed call ticketcache GetNodeTicketsCacheMap, the Global ticketTemp instance is nil !!!!!!!!!!!!!!!")
=======
		log.Warn("Warn call ticketcache GetNodeTicketsCacheMap, the Global ticketTemp instance is nil !!!!!!!!!!!!!!!")
>>>>>>> localdev/develop
	}
	return
}

func GetTicketidsCachePtr() *TicketTempCache {
	return ticketTemp
}

////////////////////////////
<<<<<<< HEAD
func  Hash(cache TicketCache) (common.Hash, error) {
=======
func Hash(cache TicketCache) (common.Hash, error) {
>>>>>>> localdev/develop

	timer := Timer{}
	timer.Begin()
	out, err := proto.Marshal(cache.GetSortStruct())
	if err != nil {
		log.Error("Faile to call ticketcache Hash", ErrProbufMarshal.Error()+":err", err)
		return common.Hash{}, err
	}
	log.Info("Call ticketcache Hash ...", "lenOut: ", len(out))
	ret := crypto.Keccak256Hash(out)
	log.Info("Call ticketcache Hash finish...", "run time  ms: ", timer.End())
	return ret, nil
}

func (t *TicketTempCache) GetNodeTicketsMap(blocknumber *big.Int, blockhash common.Hash) TicketCache {
	t.lock.Lock()
	//defer t.lock.Unlock()

	log.Info("Call TicketTempCache GetNodeTicketsMap ...", "blocknumber: ", blocknumber, " blockhash: ", blockhash.Hex())

<<<<<<< HEAD
=======
	notGenesisBlock := blocknumber.Cmp(big.NewInt(0)) > 0

>>>>>>> localdev/develop
	// a map （blocknumber => map[blockHash]map[nodeId][]ticketId）
	blockNodes, ok := t.Cache.NBlocks[blocknumber.String()]
	if !ok {
		blockNodes = &BlockNodes{}
		// create a new map （map[blockHash]map[nodeId][]ticketId）
		blockNodes.BNodes = make(map[string]*NodeTicketIds)
		// set to cache by current map （map[blockHash]map[nodeId][]ticketId）
		t.Cache.NBlocks[blocknumber.String()] = blockNodes
<<<<<<< HEAD
		log.Error("Failed to GetNodeTicketsMap, TicketCache is empty by blocknumber", "blocknumber", blocknumber.String(), "blockHash", blockhash.String())
=======
		if notGenesisBlock {
			log.Error("Failed to GetNodeTicketsMap, TicketCache is empty by blocknumber", "blocknumber", blocknumber.String(), "blockHash", blockhash.String())
		}

>>>>>>> localdev/develop
	}

	// a map (blockHash => map[nodeId][]ticketId)
	nodeTicketIds, ok := blockNodes.BNodes[blockhash.String()]
	if !ok {
		nodeTicketIds = &NodeTicketIds{}
		// create a new map (map[nodeId][]ticketId)
		nodeTicketIds.NTickets = make(map[string]*TicketIds)
		// set to cache by current map (map[nodeId][]ticketId)
		blockNodes.BNodes[blockhash.String()] = nodeTicketIds
<<<<<<< HEAD
		log.Error("Failed to GetNodeTicketsMap, TicketCache is empty by blockHash", "blocknumber", blocknumber.String(), "blockHash", blockhash.String())
	}

	if nil == nodeTicketIds.NTickets || len(nodeTicketIds.NTickets) == 0 {
		log.Warn("Warn to GetNodeTicketsMap, TicketCache'NTickets is empty", "blocknumber", blocknumber.String(), "blockHash", blockhash.String())
=======

		if notGenesisBlock {
			log.Error("Failed to GetNodeTicketsMap, TicketCache is empty by blockHash", "blocknumber", blocknumber.String(), "blockHash", blockhash.String())
		}
	}

	if nil == nodeTicketIds.NTickets || len(nodeTicketIds.NTickets) == 0 {

		if notGenesisBlock {
			log.Error("Warn to GetNodeTicketsMap, TicketCache'NTickets is empty", "blocknumber", blocknumber.String(), "blockHash", blockhash.String())
		}
>>>>>>> localdev/develop
	}

	/**
	goroutine task
	build data by global cache （map[nodeId][]ticketId）
<<<<<<< HEAD
	 */
=======
	*/
>>>>>>> localdev/develop
	type result struct {
		key  discover.NodeID
		tids []common.Hash
	}
	resCh := make(chan *result, len(nodeTicketIds.NTickets))
	var wg sync.WaitGroup
	wg.Add(len(nodeTicketIds.NTickets))

	// key == nodeId
	// value == []ticketId
	for k, v := range nodeTicketIds.NTickets {
		nid, err := discover.HexID(k)
		if err == nil {
			// copy nodeId => []tickId by routine task
			go func(nodeid discover.NodeID, tidslice *TicketIds) {
				// create a new []ticketId
				tids := make([]common.Hash, 0, len(tidslice.TicketId))

				// recursive to build ticketId  from global slice of ticketId
				for _, tid := range tidslice.TicketId {
					tids = append(tids, common.BytesToHash(tid))
				}
				res := new(result)
				res.key = nodeid
				res.tids = tids
				resCh <- res
				wg.Done()
			}(nid, v)
		} else {
			wg.Done()
<<<<<<< HEAD
			log.Error("Failed to TicketTempCache GetNodeTicketsMap: nodeId to discover.HexID error ", "NodeId: ", k, "blocknumber", blocknumber.String(), "blockHash", blockhash.String())
=======
			log.Trace("Failed to TicketTempCache GetNodeTicketsMap: nodeId to discover.HexID error ", "NodeId: ", k, "blocknumber", blocknumber.String(), "blockHash", blockhash.String())
>>>>>>> localdev/develop
		}
	}
	wg.Wait()
	close(resCh)

	t.lock.Unlock()

	/**
	Build a new TicketCache
	This TicketCache will be used in StateDB
<<<<<<< HEAD
	 */
=======
	*/
>>>>>>> localdev/develop
	out := NewTicketCache()
	for res := range resCh {
		// a map type as nodeId => []ticketId
		out[res.key] = res.tids
	}
	return out
}

func (t *TicketTempCache) Submit2Cache(blocknumber, blockInterval *big.Int, blockhash common.Hash, in map[discover.NodeID][]common.Hash) {
	t.lock.Lock()
	//defer t.lock.Unlock()

	log.Info("Call TicketTempCache Submit2Cache ", "blocknumber: ", blocknumber.String(), " blockInterval: ", blockInterval, " blockhash: ", blockhash.Hex(), " Before Submit2Cache, then cachelen: ", len(t.Cache.NBlocks))
	blockNodes, ok := t.Cache.NBlocks[blocknumber.String()]
	if !ok {
		blockNodes = &BlockNodes{}
		blockNodes.BNodes = make(map[string]*NodeTicketIds)
	}
	//The same block hash data will be overwritten
	nodeTicketIds := &NodeTicketIds{}
	nodeTicketIds.NTickets = make(map[string]*TicketIds)
	//goroutine task
	type result struct {
		key   discover.NodeID
		value *TicketIds
	}
	resCh := make(chan *result, len(in))
	var wg sync.WaitGroup
	wg.Add(len(in))
	for k, v := range in {
		go func(key discover.NodeID, val []common.Hash) {
			tIds := &TicketIds{}
			for _, va := range val {
				tIds.TicketId = append(tIds.TicketId, va.Bytes())
			}
			res := new(result)
			res.key = key
			res.value = tIds
			resCh <- res
			wg.Done()
		}(k, v)
	}
	wg.Wait()
	close(resCh)
	for res := range resCh {
		nodeTicketIds.NTickets[res.key.String()] = res.value
	}
	blockNodes.BNodes[blockhash.String()] = nodeTicketIds
	t.Cache.NBlocks[blocknumber.String()] = blockNodes

	//// tmp fix TODO
	//if big.NewInt(0).Cmp(blockInterval) > 0 {
	//	log.Error("WARN WARN WARN !!! Call TicketTempCache Submit2Cache FINISH !!!!!! blockInterval is NEGATIVE NUMBER", "blocknumber: ", blocknumber.String(), " blockInterval: ", blockInterval, " blockhash: ", blockhash.Hex(), " After Submit2Cache, then cachelen: ", len(t.Cache.NBlocks))
	//	return
	//}
	//
	//interval := new(big.Int).Add(blockInterval, big.NewInt(20))
	//
	////del old cache
	//number := new(big.Int).Sub(blocknumber, interval)
	//for k := range t.Cache.NBlocks {
	//	if n, b := new(big.Int).SetString(k, 0); b {
	//		if n.Cmp(number) < 0 {
	//			delete(t.Cache.NBlocks, k)
	//		}
	//	}
	//}
	log.Info("Call TicketTempCache Submit2Cache FINISH !!!!!! ", "blocknumber: ", blocknumber.String(), " blockInterval: ", blockInterval, " blockhash: ", blockhash.Hex(), " After Submit2Cache, then cachelen: ", len(t.Cache.NBlocks))

	t.lock.Unlock()
}

func (t *TicketTempCache) Commit(db ethdb.Database, currentBlockNumber *big.Int) error {
	t.lock.Lock()
	//defer t.lock.Unlock()
	log.Info("Call TicketTempCache Commit ...")

	timer := Timer{}
	timer.Begin()

	// TODO tmp fix
	interval := new(big.Int).Sub(currentBlockNumber, big.NewInt(30))
	log.Info("Call TicketTempCache Commit, Delete Global TicketIdsTemp key by", "currentBlockNumber", currentBlockNumber, "after calc interval", interval)
	for k := range t.Cache.NBlocks {
		if n, b := new(big.Int).SetString(k, 0); b {
			if n.Cmp(interval) < 0 {
				delete(t.Cache.NBlocks, k)
			}
		}
	}

	log.Info("Call TicketTempCache Commit, Delete Global TicketIdsTemp key FINISH !!!!", "currentBlockNumber", currentBlockNumber, "remian size after delete, then cachelen: ", len(t.Cache.NBlocks))

	out, err := proto.Marshal(t.Cache)

	if err != nil {
		log.Error("Failted to TicketPoolCache Commit ", "ErrProbufMarshal: err", err.Error())
		t.lock.Unlock()
		return ErrProbufMarshal
	}
	log.Info("Call TicketPoolCache Commit ", "cachelen: ", len(t.Cache.NBlocks), " outlen: ", len(out))
	t.lock.Unlock()

	if err := db.Put(ticketPoolCacheKey, out); err != nil {
		log.Error("Failed to call TicketPoolCache Commit: level db put faile: ", "err", err.Error())
		return ErrLeveldbPut
	}
	log.Info("Call TicketPoolCache Commit run time ", "ms: ", timer.End())
	return nil
}

func NewTicketCache() TicketCache {
	return make(TicketCache)
}

func (tc TicketCache) AppendTicketCache(nodeid discover.NodeID, tids []common.Hash) {
	value, ok := tc[nodeid]
	if !ok {
		log.Warn("Warn to AppendTicketCache, the ticketIds is empty !!!!", "nodeId", nodeid.String())
		value = make([]common.Hash, 0)
	}
	value = append(value, tids...)
	tc[nodeid] = value
}

func (tc TicketCache) GetTicketCache(nodeid discover.NodeID) ([]common.Hash, error) {
	tids, ok := tc[nodeid]
	if !ok {
<<<<<<< HEAD
		log.Error("Failed to GetTicketCache, the ticketIds is empty !!!!", "nodeId", nodeid.String())
=======
		log.Warn("Warn to GetTicketCache, the ticketIds is empty !!!!", "nodeId", nodeid.String())
>>>>>>> localdev/develop
		return nil, ErrNotfindFromNodeId
	}
	ret := make([]common.Hash, len(tids))
	copy(ret, tids)
	return ret, nil
}

func (tc TicketCache) RemoveTicketCache(nodeid discover.NodeID, tids []common.Hash) error {
	cache, ok := tc[nodeid]
	if !ok {
<<<<<<< HEAD
		log.Error("Failed to RemoveTicketCache, the ticketIds is empty !!!!", "nodeId", nodeid.String())
=======
		log.Warn("Warn to RemoveTicketCache, the ticketIds is empty !!!!", "nodeId", nodeid.String())
>>>>>>> localdev/develop
		return ErrNotfindFromNodeId
	}
	mapTIds := make(map[common.Hash]common.Hash)
	for _, id := range tids {
		mapTIds[id] = id
	}
	for i := 0; i < len(cache); i++ {
		if _, ok := mapTIds[cache[i]]; ok {
			cache = append(cache[:i], cache[i+1:]...)
			i--
		}
	}
	if len(cache) > 0 {
		tc[nodeid] = cache
	} else {
		delete(tc, nodeid)
	}
	return nil
}

func (tc TicketCache) TCount(nodeid discover.NodeID) uint64 {
	return uint64(len(tc[nodeid]))
}

// copy a cache as (nodeId => []ticketId)
func (tc TicketCache) TicketCaceheSnapshot() TicketCache {

	// create a new cache
	ret := NewTicketCache()

	// copy data from origin cache
	for nodeid, tids := range tc {

		// []ticketId
		arr := make([]common.Hash, len(tids))
		copy(arr, tids)
		ret[nodeid] = arr
	}
	return ret
}

func (tc TicketCache) GetSortStruct() *SortCalcHash {
	sc := &SortCalcHash{}
	sc.Nodeids = make([]string, 0, len(tc))
	sc.Tids = make([]*TicketIds, 0, len(tc))
	for k := range tc {
		sc.Nodeids = append(sc.Nodeids, k.String())
	}
	sort.Strings(sc.Nodeids)
	for _, k := range sc.Nodeids {
		nodeid, err := discover.HexID(k)
		if err == nil {
			tids := &TicketIds{}
			tids.TicketId = make([][]byte, 0, len(tc[nodeid]))
			for _, tid := range tc[nodeid] {
				tids.TicketId = append(tids.TicketId, tid.Bytes())
			}
			sc.Tids = append(sc.Tids, tids)
		} else {
<<<<<<< HEAD
			log.Error("Failed to TicketCache GetSortStruct: discover.HexID error ",  "err", err, "hex: ", k)
=======
			log.Error("Failed to TicketCache GetSortStruct: discover.HexID error ", "err", err, "hex: ", k)
>>>>>>> localdev/develop
		}
	}
	return sc
}
