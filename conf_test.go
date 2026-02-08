package main

import (
	"encoding/json"
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"path/filepath"
	"testing"
)

func TestConf(t *testing.T) {
	origConfFile := ConfFile
	ConfFile = filepath.Join(t.TempDir(), "conf.json")
	defer func() {
		ConfFile = origConfFile
	}()

	Convey("TestConf", t, func() {
		// load
		conf, err := LoadConfFromFile()
		So(err, ShouldBeNil)

		conf.ReadwiseToken = "your-token"
		conf.EnabledWebsites = []string{"the_initium"}

		// save
		err = conf.WriteDisk()
		So(err, ShouldBeNil)

		exported, err := conf.Export()
		So(err, ShouldBeNil)
		j, err := json.MarshalIndent(exported, "", "\t")
		So(err, ShouldBeNil)
		fmt.Print(string(j))
	})
}
