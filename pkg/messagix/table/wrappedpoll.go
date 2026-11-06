package table

import (
	"cmp"
	"slices"
)

type PollOption struct {
	OptionID int64  `json:"id"`
	Text     string `json:"text"`
}

type PollData struct {
	PollID         int64
	Options        []PollOption
	Votes          map[int64][]int64
	VoteTimestamps map[int64]int64
}

func (table *LSTable) WrapPollData() []*PollData {
	byID := make(map[int64]*PollData)
	var polls []*PollData
	for _, placeholder := range table.LSHandlePlaceholderPollData {
		if byID[placeholder.PollID] == nil {
			byID[placeholder.PollID] = &PollData{
				PollID:         placeholder.PollID,
				Votes:          make(map[int64][]int64),
				VoteTimestamps: make(map[int64]int64),
			}
			polls = append(polls, byID[placeholder.PollID])
		}
	}
	options := slices.Clone(table.LSAddPollOption)
	slices.SortStableFunc(options, func(a, b *LSAddPollOption) int {
		return cmp.Compare(a.SortKeyCreationTimestamp, b.SortKeyCreationTimestamp)
	})
	for _, opt := range options {
		if data := byID[opt.PollID]; data != nil {
			data.Options = append(data.Options, PollOption{OptionID: opt.OptionID, Text: opt.OptionText})
		}
	}
	for _, vote := range table.LSAddPollVote {
		if data := byID[vote.PollID]; data != nil {
			data.Votes[vote.ContactID] = append(data.Votes[vote.ContactID], vote.OptionID)
			data.VoteTimestamps[vote.ContactID] = max(data.VoteTimestamps[vote.ContactID], vote.TimestampMS)
		}
	}
	for _, data := range polls {
		for id, selection := range data.Votes {
			slices.Sort(selection)
			data.Votes[id] = slices.Compact(selection)
		}
	}
	return slices.DeleteFunc(polls, func(data *PollData) bool { return len(data.Options) == 0 })
}
