package firehose

import (
	"fmt"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
)

// DeferredCallState holds state changes that need to be attached to a call
// after certain operations complete. This handles edge cases where state changes
// occur outside normal call boundaries (e.g., during contract creation).
type DeferredCallState struct {
	accountCreations []*pbeth.AccountCreation
	balanceChanges   []*pbeth.BalanceChange
	gasChanges       []*pbeth.GasChange
	nonceChanges     []*pbeth.NonceChange
	codeChanges      []*pbeth.CodeChange
}

// NewDeferredCallState creates a new empty deferred call state
func NewDeferredCallState() *DeferredCallState {
	return &DeferredCallState{}
}

// IsEmpty returns true if there are no deferred state changes
func (d *DeferredCallState) IsEmpty() bool {
	return len(d.accountCreations) == 0 &&
		len(d.balanceChanges) == 0 &&
		len(d.gasChanges) == 0 &&
		len(d.nonceChanges) == 0 &&
		len(d.codeChanges) == 0
}

// Reset clears all deferred state
func (d *DeferredCallState) Reset() {
	d.accountCreations = nil
	d.balanceChanges = nil
	d.gasChanges = nil
	d.nonceChanges = nil
	d.codeChanges = nil
}

// AddAccountCreation adds an account creation to deferred state
func (d *DeferredCallState) AddAccountCreation(creation *pbeth.AccountCreation) {
	d.accountCreations = append(d.accountCreations, creation)
}

// AddBalanceChange adds a balance change to deferred state
func (d *DeferredCallState) AddBalanceChange(change *pbeth.BalanceChange) {
	d.balanceChanges = append(d.balanceChanges, change)
}

// AddGasChange adds a gas change to deferred state
func (d *DeferredCallState) AddGasChange(change *pbeth.GasChange) {
	d.gasChanges = append(d.gasChanges, change)
}

// AddNonceChange adds a nonce change to deferred state
func (d *DeferredCallState) AddNonceChange(change *pbeth.NonceChange) {
	d.nonceChanges = append(d.nonceChanges, change)
}

// AddCodeChange adds a code change to deferred state
func (d *DeferredCallState) AddCodeChange(change *pbeth.CodeChange) {
	d.codeChanges = append(d.codeChanges, change)
}

// MaybePopulateCallAndReset populates the call with deferred state if any exists
// and then resets the deferred state. This should only be called for root calls.
func (d *DeferredCallState) MaybePopulateCallAndReset(source string, call *pbeth.Call) error {
	if d.IsEmpty() {
		return nil
	}

	if source != "root" {
		return fmt.Errorf("unexpected source for deferred call state, expected root but got %s, deferred call's state are always produced on the 'root' call", source)
	}

	// Append all deferred state to the call
	call.AccountCreations = append(call.AccountCreations, d.accountCreations...)
	call.BalanceChanges = append(call.BalanceChanges, d.balanceChanges...)
	call.GasChanges = append(call.GasChanges, d.gasChanges...)
	call.NonceChanges = append(call.NonceChanges, d.nonceChanges...)
	call.CodeChanges = append(call.CodeChanges, d.codeChanges...)

	d.Reset()

	return nil
}
