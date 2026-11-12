package main

import (
	"errors"
	"fmt"
	"testing"
)

func TestInvalidFBTokenStopsRetryWithoutStoppingNetworkRetries(t *testing.T) {
	for _, message := range []string{
		"LoginRentalServer: Login: 无效的FBToken, 原因是 FBToken不存在: record not found",
		"invalid FBToken",
		"POST https://user.aurelrune.com/api/phoenix/login: HTTP 404: Unknown error",
	} {
		if !retryShouldStop(fmt.Errorf("未能连接机器人: %w", errors.New(message))) {
			t.Errorf("permanent token error would be retried: %s", message)
		}
	}
	for _, message := range []string{"connection reset by peer", "i/o timeout", "record not found"} {
		if retryShouldStop(errors.New(message)) {
			t.Errorf("unrelated failure cannot be retried: %s", message)
		}
	}
}
