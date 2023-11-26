package channel

import "github.com/sjy-dv/IZONE/pkg/log"

var InfoCh = make(chan string)
var WarnCh = make(chan string)
var ErrCh = make(chan error)

func On() {
	for {
		select {
		case info := <-InfoCh:
			log.Info(info)
		case err := <-ErrCh:
			log.Error(err)
		case warn := <-WarnCh:
			log.Warn(warn)
		}
	}
}
