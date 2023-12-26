package messagix_test

import (
	"log"
	"os"
	"testing"

	"github.com/0xzer/messagix"
	"github.com/0xzer/messagix/debug"
	"github.com/0xzer/messagix/types"
)

func TestParseJS(t *testing.T) {
	cli, err := messagix.NewClient(types.Instagram, nil, debug.NewLogger(), "")
	if err != nil {
		log.Fatal(err)
	}
	parser := &messagix.ModuleParser{}
	testData, _ := os.ReadFile("test_files/res.html")
	parser.SetTestData(testData)
	parser.SetClientInstance(cli)
	parser.Load("")
	//parser.Load("https://www.instagram.com/accounts/login/")
}