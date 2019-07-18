package plugin

import (
	"errors"
	"sync"

	"github.com/PlatONnetwork/PlatON-Go/common/byteutil"

	"github.com/PlatONnetwork/PlatON-Go/common"
	"github.com/PlatONnetwork/PlatON-Go/core/types"
	"github.com/PlatONnetwork/PlatON-Go/log"
	"github.com/PlatONnetwork/PlatON-Go/p2p/discover"
	"github.com/PlatONnetwork/PlatON-Go/x/gov"
	"github.com/PlatONnetwork/PlatON-Go/x/xcom"
	"github.com/PlatONnetwork/PlatON-Go/x/xutil"
)

var (
	govPluginOnce sync.Once
)

type GovPlugin struct {
	govDB *gov.GovDB
}

var govp *GovPlugin

func GovPluginInstance() *GovPlugin {
	govPluginOnce.Do(func() {
		govp = &GovPlugin{govDB: gov.GovDBInstance()}
	})
	//if nil == govp {
	//	govp = &GovPlugin{govDB: gov.GovDBInstance()}
	//}
	return govp
}

//func ClearGovPlugin() error {
//	if nil == govp {
//		return common.NewSysError("the GovPlugin already be nil")
//	}
//	govp = nil
//	return nil
//}

func (govPlugin *GovPlugin) Confirmed(block *types.Block) error {
	return nil
}

//implement BasePlugin
func (govPlugin *GovPlugin) BeginBlock(blockHash common.Hash, header *types.Header, state xcom.StateDB) error {

	log.Debug("call BeginBlock()", "blockNumber", header.Number.Uint64())
	if xutil.IsSettlementPeriod(header.Number.Uint64()) {
		log.Debug("current block is the end of settlement period", "blockNumber", header.Number.Uint64())
		verifierList, err := stk.ListVerifierNodeID(blockHash, header.Number.Uint64())
		if err != nil {
			return err
		}

		votingProposalIDs, err := govPlugin.govDB.ListVotingProposal(blockHash, state)
		if err != nil {
			return err
		}
		for _, votingProposalID := range votingProposalIDs {
			if err := govPlugin.govDB.AccuVerifiers(blockHash, votingProposalID, verifierList); err != nil {
				return err
			}
		}
	}
	return nil
}

func inNodeList(proposer discover.NodeID, vList []discover.NodeID) bool {
	for _, v := range vList {
		if proposer == v {
			return true
		}
	}
	return false
}

//implement BasePlugin
func (govPlugin *GovPlugin) EndBlock(blockHash common.Hash, header *types.Header, state xcom.StateDB) error {
	log.Debug("call EndBlock()", "blockNumber", header.Number.Uint64(), "blockHash", blockHash)

	votingProposalIDs, err := govPlugin.govDB.ListVotingProposal(blockHash, state)
	if err != nil {
		return err
	}

	for _, votingProposalID := range votingProposalIDs {
		log.Debug("iterate voting proposals", "proposalID", votingProposalID)
		votingProposal, err := govPlugin.govDB.GetExistProposal(votingProposalID, state)
		if nil != err {
			return err
		}
		if votingProposal.GetEndVotingBlock() == header.Number.Uint64() {
			log.Debug("current block is end-voting-block", "blockNumber", header.Number.Uint64())
			if xutil.IsSettlementPeriod(header.Number.Uint64()) {
				log.Debug("current block is the end of settlement period", "blockNumber", header.Number.Uint64())
				verifierList, err := stk.ListVerifierNodeID(blockHash, header.Number.Uint64())
				if err != nil {
					return err
				}

				if err := govPlugin.govDB.AccuVerifiers(blockHash, votingProposalID, verifierList); err != nil {
					return err
				}
			}
			if votingProposal.GetProposalType() == gov.Text {
				_, err := govPlugin.tallyBasic(votingProposal.GetProposalID(), blockHash, state)
				if err != nil {
					return err
				}
			} else if votingProposal.GetProposalType() == gov.Version {
				err := govPlugin.tallyForVersionProposal(votingProposal.(gov.VersionProposal), blockHash, header.Number.Uint64(), state)
				if err != nil {
					return err
				}
			} else if votingProposal.GetProposalType() == gov.Param {
				pass, err := govPlugin.tallyBasic(votingProposal.GetProposalID(), blockHash, state)
				if err != nil {
					return err
				}
				if pass {
					if err := govPlugin.updateParam(votingProposal.(gov.ParamProposal), blockHash, state); err != nil {
						return err
					}
				}
			} else {
				log.Error("invalid proposal type", "type", votingProposal.GetProposalType())
				err = errors.New("invalid proposal type")
				return err
			}
		}
	}

	preActiveProposalID, err := govPlugin.govDB.GetPreActiveProposalID(blockHash, state)
	if err != nil {
		log.Error("check if there's a preactive proposal failed.", "blockHash", blockHash)
		return err
	}
	if preActiveProposalID == common.ZeroHash {
		return nil
	}

	//handle a PreActiveProposal
	proposal, err := govPlugin.govDB.GetProposal(preActiveProposalID, state)
	if err != nil {
		return err
	}
	versionProposal, ok := proposal.(gov.VersionProposal)

	if ok {
		log.Debug("found pre-active version proposal", "proposalID", preActiveProposalID, "blockNumber", header.Number.Uint64(), "activeBlockNumber", versionProposal.GetActiveBlock())
		sub := header.Number.Uint64() - versionProposal.GetActiveBlock()
		if sub >= 0 && sub%xutil.ConsensusSize() == 0 {
			validatorList, err := stk.ListCurrentValidatorID(blockHash, header.Number.Uint64())
			if err != nil {
				log.Error("list current round validators failed.", "blockHash", blockHash, "blockNumber", header.Number.Uint64())
				return err
			}
			var updatedNodes uint64 = 0

			//all active validators
			activeList, err := govPlugin.govDB.GetActiveNodeList(blockHash, preActiveProposalID)
			if err != nil {
				log.Error("list all active nodes failed.", "blockHash", blockHash, "preActiveProposalID", preActiveProposalID)
				return err
			}

			//check if all validators are active
			for _, val := range validatorList {
				if inNodeList(val, activeList) {
					updatedNodes++
				}
			}
			if updatedNodes == xcom.ConsValidatorNum() {

				log.Debug("the pre-active version proposal has passed")

				tallyResult, err := govPlugin.govDB.GetTallyResult(preActiveProposalID, state)
				if err != nil {
					log.Error("find tally result by proposal ID failed.", "preActiveProposalID", preActiveProposalID)
					return err
				}
				//change tally status to "active"
				tallyResult.Status = gov.Active
				if err := govPlugin.govDB.SetTallyResult(*tallyResult, state); err != nil {
					log.Error("update tally result failed.", "preActiveProposalID", preActiveProposalID)
					return err
				}
				if err = govPlugin.govDB.MovePreActiveProposalIDToEnd(blockHash, preActiveProposalID, state); err != nil {
					log.Error("move proposal ID from preActiveProposal list to endProposal list failed.", "blockHash", blockHash, "preActiveProposalID", preActiveProposalID)
					return err
				}

				if err = govPlugin.govDB.ClearActiveNodes(blockHash, preActiveProposalID); err != nil {
					log.Error("clear active nodes failed.", "blockHash", blockHash, "preActiveProposalID", preActiveProposalID)
					return err
				}

				if err = govPlugin.govDB.SetActiveVersion(versionProposal.NewVersion, state); err != nil {
					log.Error("save active version to stateDB failed.", "blockHash", blockHash, "preActiveProposalID", preActiveProposalID)
					return err
				}
				log.Debug("PlatON is ready to upgrade to new version.")
			}
		}
	}
	return nil
}

// nil is allowed
func (govPlugin *GovPlugin) GetPreActiveVersion(state xcom.StateDB) uint32 {
	return govPlugin.govDB.GetPreActiveVersion(state)
}

// should not be a nil value
func (govPlugin *GovPlugin) GetActiveVersion(state xcom.StateDB) uint32 {
	return govPlugin.govDB.GetActiveVersion(state)
}

// submit a proposal
func (govPlugin *GovPlugin) Submit(from common.Address, proposal gov.Proposal, blockHash common.Hash, state xcom.StateDB) error {

	hex := common.Bytes2Hex(blockHash.Bytes())
	log.Debug("check sender", "blockHash", hex, "blockNumber", proposal.GetSubmitBlock())

	//param check
	if err := proposal.Verify(proposal.GetSubmitBlock(), state); err != nil {
		log.Error("verify proposal parameters failed", "err", err)
		return common.NewBizError(err.Error())
	}

	//check caller and proposer
	if err := govPlugin.checkVerifier(from, proposal.GetProposer(), blockHash, proposal.GetSubmitBlock()); err != nil {
		return err
	}

	//handle version proposal
	_, isVP := proposal.(gov.VersionProposal)
	if isVP {
		//another versionProposal in voting, exit.
		vp, err := govPlugin.findVotingVersionProposal(blockHash, state)
		if err != nil {
			log.Error("to find if there's a voting version proposal failed", "blockHash", blockHash)
			return err
		} else if vp != nil {
			log.Error("existing a voting version proposal.", "proposalID", vp.GetProposalID())
			return common.NewBizError("existing a version proposal at voting stage.")
		}
		//another VersionProposal in Pre-active process，exit
		proposalID, err := govPlugin.govDB.GetPreActiveProposalID(blockHash, state)
		if err != nil {
			log.Error("to check if there's a pre-active version proposal failed.", "blockHash", blockHash)
			return err
		}
		if proposalID != common.ZeroHash {
			return common.NewBizError("existing a pre-active version proposal")
		}
	}

	//handle storage
	if err := govPlugin.govDB.SetProposal(proposal, state); err != nil {
		log.Error("save proposal failed", "proposalID", proposal.GetProposalID())
		return err
	}
	if err := govPlugin.govDB.AddVotingProposalID(blockHash, proposal.GetProposalID(), state); err != nil {
		log.Error("add proposal ID to voting proposal ID list failed", "proposalID", proposal.GetProposalID())
		return err
	}
	return nil
}

// vote for a proposal
func (govPlugin *GovPlugin) Vote(from common.Address, vote gov.Vote, blockHash common.Hash, blockNumber uint64, state xcom.StateDB) error {
	if len(vote.ProposalID) == 0 || len(vote.VoteNodeID) == 0 || vote.VoteOption == 0 {
		return common.NewBizError("empty parameter detected.")
	}

	proposal, err := govPlugin.govDB.GetProposal(vote.ProposalID, state)
	if err != nil {
		log.Error("cannot find proposal by ID", "proposalID", vote.ProposalID)
		return err
	} else if proposal == nil {
		log.Error("incorrect proposal ID.", "proposalID", vote.ProposalID)
		return common.NewBizError("incorrect proposal ID.")
	}

	//check caller and voter
	if err := govPlugin.checkVerifier(from, proposal.GetProposer(), blockHash, proposal.GetSubmitBlock()); err != nil {
		return err
	}

	//voteOption range check
	if !(vote.VoteOption >= gov.Yes && vote.VoteOption <= gov.Abstention) {
		return common.NewBizError("vote option is error.")
	}

	//check if vote.proposalID is in voting
	votingIDs, err := govPlugin.listVotingProposalID(blockHash, state)
	if err != nil {
		log.Error("to list all voting proposal IDs failed", "blockHash", blockHash)
		return err
	} else if votingIDs == nil {
		log.Error("there's no voting proposal ID.", "blockHash", blockHash)
		return err
	} else {
		var isVoting = false
		for _, votingID := range votingIDs {
			if votingID == vote.ProposalID {
				isVoting = true
			}
		}
		if !isVoting {
			log.Error("proposal is not at voting stage", "proposalID", vote.ProposalID)
			return common.NewBizError("Proposal is not at voting stage.")
		}
	}

	//check if node has voted
	verifierList, err := govPlugin.govDB.ListVotedVerifier(vote.ProposalID, state)
	if err != nil {
		log.Error("list voted verifiers failed", "proposalID", vote.ProposalID)
		return err
	}

	if inNodeList(vote.VoteNodeID, verifierList) {
		log.Error("node has voted this proposal", "proposalID", vote.ProposalID, "nodeID", byteutil.PrintNodeID(vote.VoteNodeID))
		return common.NewBizError("node has voted this proposal.")
	}

	//handle storage
	if err := govPlugin.govDB.SetVote(vote.ProposalID, vote.VoteNodeID, vote.VoteOption, state); err != nil {
		log.Error("save vote failed", "proposalID", vote.ProposalID)
		return err
	}

	//the proposal is version type, so add the node ID to active node list.
	if proposal.GetProposalType() == gov.Version {
		if err := govPlugin.govDB.AddActiveNode(blockHash, vote.ProposalID, vote.VoteNodeID); err != nil {
			log.Error("add nodeID to active node list failed", "proposalID", vote.ProposalID, "nodeID", byteutil.PrintNodeID(vote.VoteNodeID))
			return err
		}
	}

	return nil
}

// node declares it's version
func (govPlugin *GovPlugin) DeclareVersion(from common.Address, declaredNodeID discover.NodeID, declaredVersion uint32, blockHash common.Hash, blockNumber uint64, state xcom.StateDB) error {

	//check caller is a Verifier or Candidate
	if err := govPlugin.checkVerifier(from, declaredNodeID, blockHash, blockNumber); err != nil {
		return err
	}

	if err := govPlugin.checkCandidate(from, declaredNodeID, blockHash, blockNumber); err != nil {
		return err
	}

	activeVersion := uint32(govPlugin.govDB.GetActiveVersion(state))
	if activeVersion <= 0 {
		return common.NewBizError("wrong active version.")
	}

	votingVP, err := govPlugin.findVotingVersionProposal(blockHash, state)
	if err != nil {
		log.Error("find if there's a voting version proposal failed", "blockHash", blockHash)
		return err
	}

	//there is a voting version proposal
	if votingVP != nil {
		if declaredVersion>>8 == activeVersion>>8 {
			//the declared version equals the current active version, notify staking immediately
			log.Debug("declared version equals active version.", "activeVersion", activeVersion, "declaredVersion", declaredVersion)
			stk.DeclarePromoteNotify(blockHash, blockNumber, declaredNodeID, declaredVersion)
		} else if declaredVersion>>8 == votingVP.GetNewVersion()>>8 {
			//the declared version equals the new version, will notify staking when the proposal is passed
			log.Debug("declared version equals the new version.", "newVersion", votingVP.GetNewVersion, "declaredVersion", declaredVersion)
			govPlugin.govDB.AddActiveNode(blockHash, votingVP.ProposalID, declaredNodeID)
		} else {
			log.Error("declared version neither equals active version nor new version.", "activeVersion", activeVersion, "newVersion", votingVP.GetNewVersion, "declaredVersion", declaredVersion)
			return common.NewBizError("declared version neither equals active version nor new version.")
		}
	} else {
		if declaredVersion>>8 == activeVersion>>8 {
			//the declared version is the current active version, notify staking immediately
			stk.DeclarePromoteNotify(blockHash, blockNumber, declaredNodeID, declaredVersion)
		} else {
			log.Error("there's no version proposal at voting stage, declared version should be active version.", "activeVersion", activeVersion, "declaredVersion", declaredVersion)
			return common.NewBizError("there's no version proposal at voting stage, declared version should be active version.")
		}
	}

	return nil
}

// client query a specified proposal
func (govPlugin *GovPlugin) GetProposal(proposalID common.Hash, state xcom.StateDB) (gov.Proposal, error) {
	proposal, err := govPlugin.govDB.GetProposal(proposalID, state)
	if err != nil {
		log.Error("get proposal by ID failed", "proposalID", proposalID, "msg", err.Error())
		return nil, err
	}
	if proposal == nil {
		return nil, common.NewBizError("incorrect proposal ID.")
	}
	return proposal, nil
}

// query a specified proposal's tally result
func (govPlugin *GovPlugin) GetTallyResult(proposalID common.Hash, state xcom.StateDB) (*gov.TallyResult, error) {
	tallyResult, err := govPlugin.govDB.GetTallyResult(proposalID, state)
	if err != nil {
		log.Error("get tallyResult by proposal ID failed.", "proposalID", proposalID, "msg", err.Error())
		return nil, err
	}
	if nil == tallyResult {
		return nil, common.NewBizError("get tallyResult by proposal ID failed.")
	}

	return tallyResult, nil
}

// query proposal list
func (govPlugin *GovPlugin) ListProposal(blockHash common.Hash, state xcom.StateDB) ([]gov.Proposal, error) {
	var proposalIDs []common.Hash
	var proposals []gov.Proposal

	votingProposals, err := govPlugin.govDB.ListVotingProposal(blockHash, state)
	if err != nil {
		log.Error("list voting proposals failed.", "blockHash", blockHash)
		return nil, err
	}
	endProposals, err := govPlugin.govDB.ListEndProposalID(blockHash, state)
	if err != nil {
		log.Error("list end proposals failed.", "blockHash", blockHash)
		return nil, err
	}

	preActiveProposals, err := govPlugin.govDB.GetPreActiveProposalID(blockHash, state)
	if err != nil {
		log.Error("find pre-active proposal failed.", "blockHash", blockHash)
		return nil, err
	}

	proposalIDs = append(proposalIDs, votingProposals...)
	proposalIDs = append(proposalIDs, endProposals...)
	if preActiveProposals != common.ZeroHash {
		proposalIDs = append(proposalIDs, preActiveProposals)
	}

	for _, proposalID := range proposalIDs {
		proposal, err := govPlugin.govDB.GetExistProposal(proposalID, state)
		if err != nil {
			log.Error("find proposal failed.", "proposalID", proposalID)
			return nil, err
		}
		proposals = append(proposals, proposal)
	}
	return proposals, nil
}

// tally for a text proposal
func (govPlugin *GovPlugin) tallyForTextProposal(votedVerifierList []discover.NodeID, accuCnt uint16, proposal gov.TextProposal, blockHash common.Hash, state xcom.StateDB) error {

	proposalID := proposal.ProposalID
	verifiersCnt, err := govPlugin.govDB.AccuVerifiersLength(blockHash, proposalID)
	if err != nil {
		log.Error("count accumulated verifiers failed", "proposalID", proposalID, "blockHash", blockHash)
		return err
	}

	status := gov.Voting
	yeas := uint16(0)
	nays := uint16(0)
	abstentions := uint16(0)

	voteList, err := govPlugin.govDB.ListVoteValue(proposal.ProposalID, state)
	if err != nil {
		log.Error("list voted value failed.", "blockHash", blockHash)
		return err
	}
	for _, v := range voteList {
		if v.VoteOption == gov.Yes {
			yeas++
		}
		if v.VoteOption == gov.No {
			nays++
		}
		if v.VoteOption == gov.Abstention {
			abstentions++
		}
	}
	supportRate := float64(yeas) / float64(accuCnt)

	if supportRate >= xcom.SupportRateThreshold() {
		status = gov.Pass
	} else {
		status = gov.Failed
	}

	tallyResult := &gov.TallyResult{
		ProposalID:    proposal.ProposalID,
		Yeas:          yeas,
		Nays:          nays,
		Abstentions:   abstentions,
		AccuVerifiers: verifiersCnt,
		Status:        status,
	}

	govPlugin.govDB.MoveVotingProposalIDToEnd(blockHash, proposal.ProposalID, state)

	if err := govPlugin.govDB.SetTallyResult(*tallyResult, state); err != nil {
		log.Error("save tally result failed", "tallyResult", tallyResult)
		return err
	}
	return nil
}

// tally for a version proposal
func (govPlugin *GovPlugin) tallyForVersionProposal(proposal gov.VersionProposal, blockHash common.Hash, blockNumber uint64, state xcom.StateDB) error {

	proposalID := proposal.ProposalID
	verifiersCnt, err := govPlugin.govDB.AccuVerifiersLength(blockHash, proposalID)
	if err != nil {
		log.Error("count accumulated verifiers failed", "proposalID", proposalID, "blockHash", blockHash)
		return err
	}

	voteList, err := govPlugin.govDB.ListVoteValue(proposalID, state)
	if err != nil {
		log.Error("list voted value failed", "proposalID", proposalID)
		return err
	}

	voteCnt := uint16(len(voteList))
	yeas := voteCnt //`voteOption` can be ignored in version proposal, set voteCount to passCount as default.

	status := gov.Voting
	supportRate := float64(yeas) * 100 / float64(verifiersCnt)
	if supportRate > xcom.SupportRateThreshold() {
		status = gov.PreActive

		activeList, err := govPlugin.govDB.GetActiveNodeList(blockHash, proposalID)
		if err != nil {
			log.Error("list active nodes failed", "blockHash", blockHash, "proposalID", proposalID)
			return err
		}
		govPlugin.govDB.MoveVotingProposalIDToPreActive(blockHash, proposalID, state)
		//todo: handle error
		stk.ProposalPassedNotify(blockHash, blockNumber, activeList, proposal.NewVersion)
	} else {
		status = gov.Failed
		govPlugin.govDB.MoveVotingProposalIDToEnd(blockHash, proposalID, state)
	}

	tallyResult := &gov.TallyResult{
		ProposalID:    proposalID,
		Yeas:          yeas,
		Nays:          0x0,
		Abstentions:   0x0,
		AccuVerifiers: verifiersCnt,
		Status:        status,
	}

	if err := govPlugin.govDB.SetTallyResult(*tallyResult, state); err != nil {
		log.Error("save tally result failed", "tallyResult", tallyResult)
		return err
	}
	return nil
}

func (govPlugin *GovPlugin) tallyBasic(proposalID common.Hash, blockHash common.Hash, state xcom.StateDB) (pass bool, err error) {
	verifiersCnt, err := govPlugin.govDB.AccuVerifiersLength(blockHash, proposalID)
	if err != nil {
		log.Error("count accumulated verifiers failed", "proposalID", proposalID, "blockHash", blockHash)
		return false, err
	}

	status := gov.Voting
	yeas := uint16(0)
	nays := uint16(0)
	abstentions := uint16(0)

	voteList, err := govPlugin.govDB.ListVoteValue(proposalID, state)
	if err != nil {
		log.Error("list voted value failed.", "blockHash", blockHash)
		return false, err
	}
	for _, v := range voteList {
		if v.VoteOption == gov.Yes {
			yeas++
		}
		if v.VoteOption == gov.No {
			nays++
		}
		if v.VoteOption == gov.Abstention {
			abstentions++
		}
	}
	supportRate := float64(yeas) / float64(verifiersCnt)

	if supportRate >= xcom.SupportRateThreshold() {
		status = gov.Pass
	} else {
		status = gov.Failed
	}

	tallyResult := &gov.TallyResult{
		ProposalID:    proposalID,
		Yeas:          yeas,
		Nays:          nays,
		Abstentions:   abstentions,
		AccuVerifiers: verifiersCnt,
		Status:        status,
	}

	govPlugin.govDB.MoveVotingProposalIDToEnd(blockHash, proposalID, state)

	if err := govPlugin.govDB.SetTallyResult(*tallyResult, state); err != nil {
		log.Error("save tally result failed", "tallyResult", tallyResult)
		return false, err
	}
	return status == gov.Pass, nil
}

// check if the node a verifier, and the caller address is same as the staking address
func (govPlugin *GovPlugin) checkVerifier(from common.Address, nodeID discover.NodeID, blockHash common.Hash, blockNumber uint64) error {
	verifierList, err := stk.GetVerifierList(blockHash, blockNumber, QueryStartNotIrr)
	if err != nil {
		log.Error("list verifiers failed", "blockHash", blockHash, "err", err)
		return err
	}

	for _, verifier := range verifierList {
		if verifier != nil && verifier.NodeId == nodeID {
			if verifier.StakingAddress == from {
				return nil
			} else {
				return common.NewBizError("tx sender should be node's staking address.")
			}
		}
	}
	return common.NewBizError("tx sender is not verifier.")
}

// check if the node a candidate, and the caller address is same as the staking address
func (govPlugin *GovPlugin) checkCandidate(from common.Address, nodeID discover.NodeID, blockHash common.Hash, blockNumber uint64) error {
	candidateList, err := stk.GetCandidateList(blockHash)
	if err != nil {
		log.Error("list candidates failed", "blockHash", blockHash)
		return err
	}

	for _, candidate := range candidateList {
		if candidate.NodeId == nodeID {
			if candidate.StakingAddress == from {
				return nil
			} else {
				return common.NewBizError("tx sender should be node's staking address.")
			}
		}
	}
	return common.NewBizError("tx sender is not candidate.")
}

// list all proposal IDs at voting stage
func (govPlugin *GovPlugin) listVotingProposalID(blockHash common.Hash, state xcom.StateDB) ([]common.Hash, error) {
	idList, err := govPlugin.govDB.ListVotingProposal(blockHash, state)
	if err != nil {
		log.Error("find voting version proposal failed", "blockHash", blockHash)
		return nil, err
	}
	return idList, nil
}

// find a version proposal at voting stage
func (govPlugin *GovPlugin) findVotingVersionProposal(blockHash common.Hash, state xcom.StateDB) (*gov.VersionProposal, error) {
	idList, err := govPlugin.govDB.ListVotingProposal(blockHash, state)
	if err != nil {
		log.Error("find voting version proposal failed", "blockHash", blockHash)
		return nil, err
	}
	for _, proposalID := range idList {
		p, err := govPlugin.govDB.GetExistProposal(proposalID, state)
		if err != nil {
			return nil, err
		}
		if p.GetProposalType() == gov.Version {
			vp := p.(gov.VersionProposal)
			return &vp, nil
		}
	}
	return nil, nil
}

func (govPlugin *GovPlugin) SetParam(paraMap map[string]interface{}, state xcom.StateDB) error {
	return govPlugin.govDB.SetParam(paraMap, state)
}

func (govPlugin *GovPlugin) ListParam(state xcom.StateDB) (map[string]interface{}, error) {
	paramList, err := govPlugin.govDB.ListParam(state)
	if err != nil {
		log.Error("list all parameters failed", "msg", err.Error())
		return nil, err
	}
	return paramList, nil
}

func (govPlugin *GovPlugin) GetParamValue(name string, state xcom.StateDB) (interface{}, error) {
	value, err := govPlugin.govDB.GetParam(name, state)
	if err != nil {
		log.Error("fina a parameter failed", "msg", err.Error())
		return nil, err
	}
	return value, nil
}

func (govPlugin *GovPlugin) updateParam(proposal gov.ParamProposal, hashes common.Hash, state xcom.StateDB) error {
	if err := govPlugin.govDB.UpdateParam(proposal.ParamName, proposal.CurrentValue, proposal.NewValue, state); err != nil {
		log.Error("update parameter value failed", "msg", err.Error())
		return err
	}
	return nil
}
