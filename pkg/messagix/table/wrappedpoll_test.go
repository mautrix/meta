package table

import (
	"reflect"
	"testing"
)

const (
	testPollID    = 100
	testThreadKey = 300
	optionRed     = 101
	optionGreen   = 102
	optionBlue    = 103
	optionYellow  = 104

	voterA = 201
	voterB = 202
	voterC = 203
	voterD = 204
)

// testPollXMA is a poll attachment, which only includes the first 3 options.
func testPollXMA() *LSInsertXmaAttachment {
	return &LSInsertXmaAttachment{
		ThreadKey:                testThreadKey,
		MessageId:                "mid.$poll",
		AttachmentFbid:           "100",
		ListItemsDescriptionText: "Favorite color?",
		ListItemId1:              optionRed,
		ListItemTitleText1:       "Red",
		ListItemId2:              optionGreen,
		ListItemTitleText2:       "Green",
		ListItemId3:              optionYellow,
		ListItemTitleText3:       "Yellow",
	}
}

func testPollCTAs() []*LSInsertAttachmentCta {
	return []*LSInsertAttachmentCta{
		{AttachmentFbid: "100", Type_: "xma_poll_details_button", Title: "Change vote"},
		{AttachmentFbid: "100", Type_: "xma_poll_details_card", Title: "Change vote"},
	}
}

// testPollDataTable contains handlePlaceholderPollData/addPollOption/
// addPollVote rows for a poll with a 4th option that the poll attachment
// doesn't include. Options are deliberately out of creation order.
func testPollDataTable() *LSTable {
	vote := func(optionID, contactID int64) *LSAddPollVote {
		return &LSAddPollVote{OptionID: optionID, PollID: testPollID, ContactID: contactID}
	}
	return &LSTable{
		LSHandlePlaceholderPollData: []*LSHandlePlaceholderPollData{{PollID: testPollID}},
		LSAddPollOption: []*LSAddPollOption{
			{OptionID: optionYellow, PollID: testPollID, OptionText: "Yellow", SortKeyCreationTimestamp: 1700000003000},
			{OptionID: optionRed, PollID: testPollID, OptionText: "Red", SortKeyCreationTimestamp: 1700000000000},
			{OptionID: optionGreen, PollID: testPollID, OptionText: "Green", SortKeyCreationTimestamp: 1700000001000},
			{OptionID: optionBlue, PollID: testPollID, OptionText: "Blue", SortKeyCreationTimestamp: 1700000002000},
		},
		LSAddPollVote: []*LSAddPollVote{
			vote(optionRed, voterA),
			vote(optionRed, voterB),
			vote(optionGreen, voterB),
			vote(optionBlue, voterC),
			vote(optionYellow, voterD),
		},
	}
}

func TestWrapPolls_ParsesPoll(t *testing.T) {
	tbl := &LSTable{
		LSInsertXmaAttachment: []*LSInsertXmaAttachment{testPollXMA(), testPollXMA()},
		LSInsertAttachmentCta: testPollCTAs(),
	}
	polls := tbl.WrapPolls()
	want := []*PollAttachment{{PollID: testPollID, ThreadKey: testThreadKey, Question: "Favorite color?"}}
	if !reflect.DeepEqual(polls, want) {
		t.Errorf("unexpected polls: %+v", polls)
	}
}

func TestWrapPolls_IgnoresNonPollAttachments(t *testing.T) {
	// A regular XMA attachment can also populate the ListItems* fields, but
	// only a paired CTA with a "xma_poll_" type marks it as a poll.
	tbl := &LSTable{
		LSInsertXmaAttachment: []*LSInsertXmaAttachment{{
			AttachmentFbid:           "1234",
			ListItemsDescriptionText: "Not actually a poll",
		}},
		LSInsertAttachmentCta: []*LSInsertAttachmentCta{
			{AttachmentFbid: "1234", Type_: "xma_web_url", Title: "Open"},
		},
	}
	if polls := tbl.WrapPolls(); len(polls) != 0 {
		t.Errorf("expected no polls, got %d", len(polls))
	}
}

func TestWrapPolls_IgnoresBlankAttachments(t *testing.T) {
	// Meta sometimes sends a blank insertXmaAttachment for a poll.
	tbl := &LSTable{
		LSInsertXmaAttachment: []*LSInsertXmaAttachment{{AttachmentFbid: "100"}},
		LSInsertAttachmentCta: testPollCTAs(),
	}
	if polls := tbl.WrapPolls(); len(polls) != 0 {
		t.Errorf("expected no polls, got %d", len(polls))
	}
}

func TestChangedPollIDs(t *testing.T) {
	tbl := &LSTable{
		LSAddPollVoteV2:    []*LSAddPollVote{{PollID: testPollID}, {PollID: 1}},
		LSRemovePollVoteV2: []*LSAddPollVote{{PollID: testPollID}, {PollID: 2}},
	}
	if ids := tbl.ChangedPollIDs(); !reflect.DeepEqual(ids, []int64{testPollID, 1, 2}) {
		t.Errorf("unexpected poll IDs: %v", ids)
	}
}

func TestWrapPollData_IncludesAllOptionsInCreationOrder(t *testing.T) {
	polls := testPollDataTable().WrapPollData()
	if len(polls) != 1 || polls[0].PollID != testPollID {
		t.Fatalf("expected data for poll %d, got %+v", testPollID, polls)
	}
	var ids []int64
	for _, opt := range polls[0].Options {
		ids = append(ids, opt.OptionID)
	}
	want := []int64{optionRed, optionGreen, optionBlue, optionYellow}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("unexpected option order\n got: %v\nwant: %v", ids, want)
	}
	wantVotes := map[int64][]int64{
		voterA: {optionRed},
		voterB: {optionRed, optionGreen},
		voterC: {optionBlue},
		voterD: {optionYellow},
	}
	if !reflect.DeepEqual(polls[0].Votes, wantVotes) {
		t.Errorf("unexpected votes: %v", polls[0].Votes)
	}
	if polls[0].CreatedAtMs != 1700000000000 {
		t.Errorf("unexpected creation time: %d", polls[0].CreatedAtMs)
	}
}

func TestWrapPollData_IgnoresOptionsWithoutPlaceholder(t *testing.T) {
	tbl := testPollDataTable()
	tbl.LSHandlePlaceholderPollData = nil
	if polls := tbl.WrapPollData(); len(polls) != 0 {
		t.Errorf("expected no poll data without handlePlaceholderPollData, got %+v", polls)
	}
}

func TestDiffPollVotes(t *testing.T) {
	previous := testPollDataTable().WrapPollData()[0]
	if changes := DiffPollVotes(nil, previous); len(changes) != 4 {
		t.Errorf("expected every voter to be new, got %+v", changes)
	}

	// voterB switches from red and green to only blue, and voterD retracts
	// their vote.
	current := previous.WithVote(voterB, []int64{optionBlue}).WithVote(voterD, nil)
	want := []PollVote{{ContactID: voterB, OptionIDs: []int64{optionBlue}}, {ContactID: voterD}}
	if changes := DiffPollVotes(previous, current); !reflect.DeepEqual(changes, want) {
		t.Errorf("unexpected changes\n got: %+v\nwant: %+v", changes, want)
	}
	if !reflect.DeepEqual(previous.Votes[voterB], []int64{optionRed, optionGreen}) {
		t.Errorf("WithVote modified the original data: %v", previous.Votes)
	}
	if len(DiffPollVotes(current, current.WithVote(voterB, []int64{optionBlue}))) != 0 {
		t.Errorf("setting the same selection again should not produce changes")
	}
}
