package bmc

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/go-multierror"
)

// PowerSetter sets the power state of a BMC
type PowerSetter interface {
	// PowerSet sets the power state of a Machine through a BMC.
	// While the state's accepted are ultimately up to what the implementation
	// expects, implementations should generally try to provide support for the following
	// states, modeled after the functionality available in `ipmitool chassis power`.
	//
	// "on": Power up chassis. should not error if the machine is already on
	// "off": Hard powers down chassis. should not error if the machine is already off
	// "soft": Initiate a soft-shutdown of OS via ACPI.
	// "reset": soft down and then power on. simulates a reboot from the host OS.
	// "cycle": hard power down followed by a power on. simulates pressing a power button
	// to turn the machine off then pressing the button again to turn it on.
	PowerSet(ctx context.Context, state string) (ok bool, err error)
}

// PowerStateGetter gets the power state of a BMC
type PowerStateGetter interface {
	PowerStateGet(ctx context.Context) (state string, err error)
}

// SetPowerState sets the power state for a BMC, trying all interface implementations passed in
func SetPowerState(ctx context.Context, state string, p []PowerSetter) (ok bool, err error) {
	var setErr error
Loop:
	for _, elem := range p {
		select {
		case <-ctx.Done():
			err = multierror.Append(err, ctx.Err())
			break Loop
		default:
			if elem != nil {
				ok, setErr = elem.PowerSet(ctx, state)
				if setErr != nil {
					err = multierror.Append(err, setErr)
					continue
				}
				if !ok {
					err = multierror.Append(err, errors.New("failed to set power state"))
					continue
				}
				return ok, nil
			}
		}
	}
	return ok, multierror.Append(err, errors.New("failed to set power state"))
}

// SetPowerStateFromInterfaces pass through to library function
func SetPowerStateFromInterfaces(ctx context.Context, state string, generic []interface{}) (ok bool, err error) {
	powerSetter := make([]PowerSetter, 0)
	for _, elem := range generic {
		switch p := elem.(type) {
		case PowerSetter:
			powerSetter = append(powerSetter, p)
		default:
			e := fmt.Sprintf("not a PowerSetter implementation: %T", p)
			err = multierror.Append(err, errors.New(e))
		}
	}
	if len(powerSetter) == 0 {
		return ok, multierror.Append(err, errors.New("no PowerSetter implementations found"))
	}
	return SetPowerState(ctx, state, powerSetter)
}

// GetPowerState sets the power state for a BMC, trying all interface implementations passed in
func GetPowerState(ctx context.Context, p []PowerStateGetter) (state string, err error) {
	var stateErr error
Loop:
	for _, elem := range p {
		select {
		case <-ctx.Done():
			err = multierror.Append(err, ctx.Err())
			break Loop
		default:
			if elem != nil {
				state, stateErr = elem.PowerStateGet(ctx)
				if stateErr != nil {
					err = multierror.Append(err, stateErr)
					continue
				}
				return state, nil
			}
		}
	}
	return state, multierror.Append(err, errors.New("failed to get power state"))
}

// GetPowerStateFromInterfaces pass through to library function
func GetPowerStateFromInterfaces(ctx context.Context, generic []interface{}) (state string, err error) {
	powerStateGetter := make([]PowerStateGetter, 0)
	for _, elem := range generic {
		switch p := elem.(type) {
		case PowerStateGetter:
			powerStateGetter = append(powerStateGetter, p)
		default:
			e := fmt.Sprintf("not a PowerStateGetter implementation: %T", p)
			err = multierror.Append(err, errors.New(e))
		}
	}
	if len(powerStateGetter) == 0 {
		return state, multierror.Append(err, errors.New("no PowerStateGetter implementations found"))
	}
	return GetPowerState(ctx, powerStateGetter)
}
