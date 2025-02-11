// Copyright Fuzamei Corp. 2018 All Rights Reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

import (
	"bytes"
	"errors"
	"strings"
	"sync"

	tmtypes "github.com/33cn/chain33/plugin/dapp/qbftNode/types"
)

// RoundVoteSet ...
type RoundVoteSet struct {
	Prevotes   *VoteSet
	Precommits *VoteSet
}

/*
Keeps track of all VoteSets from round 0 to round 'round'.

Also keeps track of up to one RoundVoteSet greater than
'round' from each peer, to facilitate catchup syncing of commits.

A commit is +2/3 precommits for a block at a round,
but which round is not known in advance, so when a peer
provides a precommit for a round greater than mtx.round,
we create a new entry in roundVoteSets but also remember the
peer to prevent abuse.
We let each peer provide us with up to 2 unexpected "catchup" rounds.
One for their LastCommit round, and another for the official commit round.
*/

// HeightVoteSet ...
type HeightVoteSet struct {
	chainID string
	height  int64
	seq     int64
	valSet  *ValidatorSet

	mtx               sync.Mutex
	round             int                  // max tracked round
	roundVoteSets     map[int]RoundVoteSet // keys: [0...round]
	peerCatchupRounds map[string][]int     // keys: peer.Key; values: at most 2 rounds
}

// NewHeightVoteSet ...
func NewHeightVoteSet(chainID string, height int64, valSet *ValidatorSet, seq int64) *HeightVoteSet {
	hvs := &HeightVoteSet{
		chainID: chainID,
		seq:     seq,
	}
	hvs.Reset(height, valSet)
	return hvs
}

// Reset ...
func (hvs *HeightVoteSet) Reset(height int64, valSet *ValidatorSet) {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()

	hvs.height = height
	hvs.valSet = valSet
	hvs.roundVoteSets = make(map[int]RoundVoteSet)
	hvs.peerCatchupRounds = make(map[string][]int)

	hvs.addRound(0)
	hvs.round = 0
}

// Height ...
func (hvs *HeightVoteSet) Height() int64 {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	return hvs.height
}

// Round ...
func (hvs *HeightVoteSet) Round() int {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	return hvs.round
}

// SetValSet update ValidatorSet
func (hvs *HeightVoteSet) SetValSet(valSet *ValidatorSet) {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	hvs.valSet = valSet
}

// SetRound Create more RoundVoteSets up to round.
func (hvs *HeightVoteSet) SetRound(round int) {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	if hvs.round != 0 && (round < hvs.round-1) {
		panic(Fmt("SetRound() must increment hvs.round %v, round %v", hvs.round, round))
	}
	for r := hvs.round - 1; r <= round; r++ {
		if rvs, ok := hvs.roundVoteSets[r]; ok {
			if r == round-1 {
				hvs.updateValSet(rvs)
			}
			continue // Already exists because peerCatchupRounds.
		}
		hvs.addRound(r)
	}
	hvs.round = round
}

func (hvs *HeightVoteSet) addRound(round int) {
	if _, ok := hvs.roundVoteSets[round]; ok {
		panic("addRound() for an existing round")
	}
	prevotes := NewVoteSet(hvs.chainID, hvs.height, round, VoteTypePrevote, hvs.valSet, hvs.seq)
	precommits := NewVoteSet(hvs.chainID, hvs.height, round, VoteTypePrecommit, hvs.valSet, hvs.seq)
	hvs.roundVoteSets[round] = RoundVoteSet{
		Prevotes:   prevotes,
		Precommits: precommits,
	}
	ttlog.Debug("addRound", "round", round, "proposer", hvs.valSet.Proposer.String())
}

func (hvs *HeightVoteSet) updateValSet(rvs RoundVoteSet) {
	hvsAddr := hvs.valSet.GetProposer().Address
	rvsAddr := rvs.Prevotes.valSet.GetProposer().Address
	if !bytes.Equal(hvsAddr, rvsAddr) {
		ttlog.Info("update RoundVoteSet proposer", "round", rvs.Prevotes.Round(),
			"old", Fmt("%X", Fingerprint(rvsAddr)), "new", Fmt("%X", Fingerprint(hvsAddr)))
		rvs.Prevotes.valSet = hvs.valSet
		rvs.Precommits.valSet = hvs.valSet
	}
}

// AddVote Duplicate votes return added=false, err=nil.
// By convention, peerKey is "" if origin is self.
func (hvs *HeightVoteSet) AddVote(vote *Vote, peerID string) (added bool, err error) {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	if !IsVoteTypeValid(byte(vote.Type)) {
		return
	}
	round := int(vote.Round)
	voteSet := hvs.getVoteSet(round, byte(vote.Type))
	if voteSet == nil {
		if rndz := hvs.peerCatchupRounds[peerID]; len(rndz) < 2 {
			hvs.addRound(int(vote.Round))
			voteSet = hvs.getVoteSet(round, byte(vote.Type))
			hvs.peerCatchupRounds[peerID] = append(rndz, round)
		} else {
			// Peer has sent a vote that does not match our round,
			// for more than one round.  Bad peer!
			// TODO punish peer.
			// log.Warn("Deal with peer giving votes from unwanted rounds")
			err = errors.New("Peer has sent a vote that does not match our round for more than one round")
			return
		}
	}
	added, err = voteSet.AddVote(vote)
	return
}

// AddAggVote Duplicate votes return added=false, err=nil.
// By convention, peerKey is "" if origin is self.
func (hvs *HeightVoteSet) AddAggVote(vote *AggVote, peerID string) (added bool, err error) {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	if !IsVoteTypeValid(byte(vote.Type)) {
		return
	}
	round := int(vote.Round)
	voteSet := hvs.getVoteSet(round, byte(vote.Type))
	if voteSet == nil {
		if rndz := hvs.peerCatchupRounds[peerID]; len(rndz) < 2 {
			hvs.addRound(int(vote.Round))
			voteSet = hvs.getVoteSet(round, byte(vote.Type))
			hvs.peerCatchupRounds[peerID] = append(rndz, round)
		} else {
			err = errors.New("Peer has sent a aggregate vote that does not match our round for more than one round")
			return
		}
	}
	added, err = voteSet.AddAggVote(vote)
	return
}

// Prevotes ...
func (hvs *HeightVoteSet) Prevotes(round int) *VoteSet {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	return hvs.getVoteSet(round, VoteTypePrevote)
}

// Precommits ...
func (hvs *HeightVoteSet) Precommits(round int) *VoteSet {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	return hvs.getVoteSet(round, VoteTypePrecommit)
}

// POLInfo Last round and blockID that has +2/3 prevotes for a particular block or nil.
// Returns -1 if no such round exists.
func (hvs *HeightVoteSet) POLInfo() (polRound int, polBlockID BlockID) {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	for r := hvs.round; r >= 0; r-- {
		rvs := hvs.getVoteSet(r, VoteTypePrevote)
		polBlockID, ok := rvs.TwoThirdsMajority()
		if ok {
			return r, BlockID{&polBlockID}
		}
	}
	return -1, BlockID{}
}

func (hvs *HeightVoteSet) getVoteSet(round int, voteType byte) *VoteSet {
	rvs, ok := hvs.roundVoteSets[round]
	if !ok {
		return nil
	}
	switch voteType {
	case VoteTypePrevote:
		return rvs.Prevotes
	case VoteTypePrecommit:
		return rvs.Precommits
	default:
		panic(Fmt("Unexpected vote type %X", voteType))
	}
}

func (hvs *HeightVoteSet) String() string {
	return hvs.StringIndented("")
}

// StringIndented ...
func (hvs *HeightVoteSet) StringIndented(indent string) string {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	vsStrings := make([]string, 0, (len(hvs.roundVoteSets)+1)*2)
	// rounds 0 ~ hvs.round inclusive
	for round := 0; round <= hvs.round; round++ {
		voteSetString := hvs.roundVoteSets[round].Prevotes.StringShort()
		vsStrings = append(vsStrings, voteSetString)
		voteSetString = hvs.roundVoteSets[round].Precommits.StringShort()
		vsStrings = append(vsStrings, voteSetString)
	}
	// all other peer catchup rounds
	for round, roundVoteSet := range hvs.roundVoteSets {
		if round <= hvs.round {
			continue
		}
		voteSetString := roundVoteSet.Prevotes.StringShort()
		vsStrings = append(vsStrings, voteSetString)
		voteSetString = roundVoteSet.Precommits.StringShort()
		vsStrings = append(vsStrings, voteSetString)
	}
	return Fmt(`HeightVoteSet{H:%v R:0~%v
%s  %v
%s}`,
		hvs.height, hvs.round,
		indent, strings.Join(vsStrings, "\n"+indent+"  "),
		indent)
}

// SetPeerMaj23 If a peer claims that it has 2/3 majority for given blockKey, call this.
// NOTE: if there are too many peers, or too much peer churn,
// this can cause memory issues.
// TODO: implement ability to remove peers too
func (hvs *HeightVoteSet) SetPeerMaj23(round int, voteType byte, peerID string, blockID *tmtypes.QbftBlockID) {
	hvs.mtx.Lock()
	defer hvs.mtx.Unlock()
	if !IsVoteTypeValid(voteType) {
		return
	}
	voteSet := hvs.getVoteSet(round, voteType)
	if voteSet == nil {
		return
	}
	voteSet.SetPeerMaj23(peerID, blockID)
}
