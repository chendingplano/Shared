package sharedtesters

import "github.com/chendingplano/shared/go/api/autotesters"

func RegisterTesters() {
	autotesters.GlobalRegistry.Register("tester_database", func() autotesters.Tester {
		return NewDatabaseTester(nil) // DB config will be set in Prepare
	})
	autotesters.GlobalRegistry.Register("tester_databaseutil", func() autotesters.Tester {
		return NewDatabaseUtilTester()
	})
	autotesters.GlobalRegistry.Register("tester_logger", func() autotesters.Tester {
		return NewLoggerTester()
	})
}
