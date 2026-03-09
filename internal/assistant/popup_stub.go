//go:build !darwin

package assistant

import "fmt"

func newPopupPresenter() (popupPresenter, error) {
	return nil, fmt.Errorf("assistant popup is unsupported on this platform")
}
