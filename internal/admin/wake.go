package admin

import (
	"fmt"
	"log"
	"time"
)

func startGroupWake(macs []string, broadcast string, gap time.Duration, sendPacket func(string, string) error) error {
	if len(macs) == 0 {
		return nil
	}
	send := func(mac string) error {
		if err := sendPacket(mac, broadcast); err != nil {
			log.Printf("ClientGroup wake: failed for %s (broadcast: %s): %v", mac, broadcast, err)
			return fmt.Errorf("%s (broadcast: %s): %w", mac, broadcast, err)
		}
		log.Printf("ClientGroup wake: sent to %s (broadcast: %s)", mac, broadcast)
		return nil
	}
	if err := send(macs[0]); err != nil {
		return err
	}
	if len(macs) > 1 {
		go func() {
			for _, mac := range macs[1:] {
				if gap > 0 {
					time.Sleep(gap)
				}
				send(mac)
			}
		}()
	}
	return nil
}
