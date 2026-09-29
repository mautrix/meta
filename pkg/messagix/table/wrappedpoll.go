package table

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
)

// PollAttachment is a poll's XMA attachment, which Meta attaches to the
// poll's creation message and every vote notification. It only has the first
// 3 options, so the full options and votes come from PollData instead.
type PollAttachment struct {
	PollID    int64
	ThreadKey int64
	Question  string
}

func (p *PollAttachment) GetThreadKey() int64 {
	return p.ThreadKey
}

// WrapPolls returns the poll attachments in this table, deduplicated by poll
// ID. An XMA is a poll if it has a question and a CTA of type "xma_poll_*".
func (table *LSTable) WrapPolls() []*PollAttachment {
	isPoll := make(map[string]bool)
	for _, cta := range table.LSInsertAttachmentCta {
		if strings.HasPrefix(cta.Type_, "xma_poll_") {
			isPoll[cta.AttachmentFbid] = true
		}
	}
	var polls []*PollAttachment
	for _, xma := range table.LSInsertXmaAttachment {
		if xma.ListItemsDescriptionText == "" || !isPoll[xma.AttachmentFbid] {
			continue
		}
		pollID, err := strconv.ParseInt(xma.AttachmentFbid, 10, 64)
		if err != nil || slices.ContainsFunc(polls, func(p *PollAttachment) bool { return p.PollID == pollID }) {
			continue
		}
		polls = append(polls, &PollAttachment{PollID: pollID, ThreadKey: xma.ThreadKey, Question: xma.ListItemsDescriptionText})
	}
	return polls
}

// ChangedPollIDs returns the IDs of polls with live vote changes in this table.
func (table *LSTable) ChangedPollIDs() []int64 {
	var ids []int64
	for _, vote := range slices.Concat(table.LSAddPollVoteV2, table.LSRemovePollVoteV2) {
		if !slices.Contains(ids, vote.PollID) {
			ids = append(ids, vote.PollID)
		}
	}
	return ids
}

type PollOption struct {
	OptionID int64
	Text     string
}

// PollData is a poll's complete options and votes, as sent in response to a
// poll_point_query task.
type PollData struct {
	PollID  int64
	Options []PollOption
	// Votes maps each voter's contact ID to their sorted selected option IDs.
	Votes map[int64][]int64
	// CreatedAtMs is the creation time of the first option.
	CreatedAtMs int64
}

// WrapPollData returns the full data of every poll in this table. Only polls
// marked with handlePlaceholderPollData are included, since other option
// rows may be partial updates.
func (table *LSTable) WrapPollData() []*PollData {
	byID := make(map[int64]*PollData)
	var polls []*PollData
	for _, placeholder := range table.LSHandlePlaceholderPollData {
		if byID[placeholder.PollID] == nil {
			byID[placeholder.PollID] = &PollData{PollID: placeholder.PollID, Votes: make(map[int64][]int64)}
			polls = append(polls, byID[placeholder.PollID])
		}
	}
	options := slices.Concat(table.LSAddPollOption, table.LSAddPollOptionV2)
	slices.SortStableFunc(options, func(a, b *LSAddPollOption) int {
		return cmp.Compare(a.SortKeyCreationTimestamp, b.SortKeyCreationTimestamp)
	})
	for _, opt := range options {
		if data := byID[opt.PollID]; data != nil {
			if len(data.Options) == 0 {
				data.CreatedAtMs = opt.SortKeyCreationTimestamp
			}
			data.Options = append(data.Options, PollOption{OptionID: opt.OptionID, Text: opt.OptionText})
		}
	}
	for _, vote := range table.LSAddPollVote {
		if data := byID[vote.PollID]; data != nil {
			data.Votes[vote.ContactID] = append(data.Votes[vote.ContactID], vote.OptionID)
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

// PollVote is one voter's complete selection. No options means the vote was retracted.
type PollVote struct {
	ContactID int64
	OptionIDs []int64
}

// DiffPollVotes returns the votes that changed from previous (which may be
// nil) to current, sorted by contact ID.
func DiffPollVotes(previous, current *PollData) []PollVote {
	var prevVotes map[int64][]int64
	if previous != nil {
		prevVotes = previous.Votes
	}
	var changes []PollVote
	for id, selection := range current.Votes {
		if !slices.Equal(selection, prevVotes[id]) {
			changes = append(changes, PollVote{ContactID: id, OptionIDs: selection})
		}
	}
	for id := range prevVotes {
		if _, ok := current.Votes[id]; !ok {
			changes = append(changes, PollVote{ContactID: id})
		}
	}
	slices.SortFunc(changes, func(a, b PollVote) int { return cmp.Compare(a.ContactID, b.ContactID) })
	return changes
}
