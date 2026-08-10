package corpus

func audit()       {}
func persist()     {}
func logEvent()    {}
func storeRecord() {}

func Original(ok bool) {
	audit()
	if ok {
		persist()
	}
}

func Renamed(flag bool) {
	logEvent()
	if flag {
		storeRecord()
	}
}

func Swapped(ok bool) {
	persist()
	if ok {
		audit()
	}
}
