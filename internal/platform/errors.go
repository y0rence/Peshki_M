package platform

import (
	"fmt"
	"strings"
)

func joinErrors(prefix string, errs []error) error {
	if len(errs) == 0 {
		return nil
	}

	messages := make([]string, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			messages = append(messages, err.Error())
		}
	}
	return fmt.Errorf("%s: %s", prefix, strings.Join(messages, "; "))
}
