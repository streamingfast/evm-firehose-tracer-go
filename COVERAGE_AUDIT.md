# Firehose Tracer Coverage Audit

## Hook Methods Overview

### Block Lifecycle Hooks
1. ✅ `OnBlockchainInit` - Initialization (tested in NewTracerTester)
2. ✅ `OnBlockStart` - Block beginning (tested extensively)
3. ❌ `OnSkippedBlock` - **NOT TESTED** - Block skipped (uncle/fork)
4. ⚠️  `OnBlockEnd` - Block ending (tested but limited block-level state changes)
5. ✅ `OnGenesisBlock` - **NOT NEEDED** (genesis only)
6. ✅ `OnClose` - Cleanup (not needed for testing)

### Transaction Lifecycle Hooks
7. ✅ `OnTxStart` - Transaction start (tested extensively)
8. ✅ `OnTxEnd` - Transaction end (tested extensively)
9. ✅ `OnSystemCallStart` - System call start (8 tests)
10. ✅ `OnSystemCallEnd` - System call end (8 tests)

### Call Lifecycle Hooks
11. ✅ `OnCallEnter` - Call entry (tested extensively)
12. ✅ `OnCallExit` - Call exit (tested extensively)
13. ⚠️  `OnOpcode` - Opcode execution (only tested for SELFDESTRUCT)
14. ❌ `OnOpcodeFault` - **NOT TESTED** - Opcode fault
15. ❌ `OnKeccakPreimage` - **NOT TESTED** - Keccak preimage

### State Change Hooks
16. ✅ `OnBalanceChange` - Balance changes (18 tests)
17. ✅ `OnNonceChange` - Nonce changes (4 tests)
18. ✅ `OnCodeChange` - Code changes (7 tests)
19. ✅ `OnStorageChange` - Storage changes (6 tests)
20. ✅ `OnLog` - Event logs (10 tests)
21. ✅ `OnGasChange` - Gas changes (18 tests)
22. ⚠️  `OnNewAccount` - **DEPRECATED** - Not tested (low priority)

## Coverage Gaps Analysis

### 1. Block-Level State Changes ✅ COMPLETED

**Current State**: Block-level state changes now fully tested in `tracer_block_level_test.go`!

**Tests Added** (14 subtests):
- ✅ **Block-level code changes** - 3 tests (deployment, update, multiple changes)
- ✅ **Miner rewards** (REASON_REWARD_MINE_BLOCK) - tested
- ✅ **Uncle rewards** (REASON_REWARD_MINE_UNCLE) - tested
- ✅ **Transaction fee rewards** (REASON_REWARD_TRANSACTION_FEE) - tested
- ✅ **Block-level balance changes** - 5 tests (single/multiple rewards, combinations)
- ✅ **Withdrawals** (EIP-4895 beacon chain withdrawals) - 4 tests
- ✅ **Complex scenarios** - 2 tests (all state types combined, multiple system calls + rewards)

**Impact**: Critical gap now resolved! All production block-level scenarios covered.

### 2. OnSkippedBlock Hook (MEDIUM GAP)

**Current State**: Not tested at all.

**Missing Tests**:
- ❌ Uncle block skipped
- ❌ Fork reorganization skipped blocks

**Impact**: Medium - Important for understanding chain reorganizations.

### 3. Edge Cases in Existing Hooks (HIGH PRIORITY)

#### OnCallEnter/OnCallExit
**Missing Edge Cases**:
- ❌ Maximum call depth (1024 calls)
- ❌ Call depth overflow/underflow
- ❌ Out of gas during call
- ❌ Precompiled contract calls (only 1 tested in integration)
- ❌ CREATE2 with collision
- ❌ DELEGATECALL with failed code lookup
- ❌ STATICCALL violation (state modification attempt)

#### OnBalanceChange
**Missing Edge Cases**:
- ❌ Extremely large balance transfers (> uint256 max)
- ❌ Negative balance (should be impossible but edge case)
- ❌ Balance overflow/underflow
- ❌ DAO hard fork balance adjustments (REASON_DAO_ADJUST_BALANCE tested but limited)

#### OnCodeChange
**Missing Edge Cases**:
- ✅ EIP-7702 delegation (tested)
- ❌ Code size limit violations (24KB)
- ❌ Empty code deployments
- ❌ Code change during failed transaction (should revert)
- ❌ Multiple code changes to same address in one block

#### OnStorageChange
**Missing Edge Cases**:
- ❌ Storage collision (same key, different values)
- ❌ Storage revert scenarios
- ❌ Storage no-op (same old/new value) - **TESTED** ✅
- ❌ Transient storage (EIP-1153) if supported

#### OnLog
**Missing Edge Cases**:
- ❌ Maximum topics (4 topics) - **TESTED** ✅
- ❌ Extremely large log data
- ❌ Logs from reverted calls (should have proper indices)
- ❌ Logs from STATICCALL violations

#### OnGasChange
**Missing Edge Cases**:
- ❌ Gas refund overflow
- ❌ Negative gas (error state)
- ❌ Gas underflow
- ❌ EIP-3529 gas refund cap scenarios

### 4. Transaction Type Coverage

**Current State**: We test legacy, access list, dynamic fee, blob, set code.

**Missing Edge Cases**:
- ❌ Invalid transaction types
- ❌ Transaction type transitions (legacy → EIP-1559 in same block)
- ❌ Blob transaction with 0 blobs
- ❌ Blob transaction with max blobs (6)

### 5. Suicide/SELFDESTRUCT Edge Cases

**Current State**: Good coverage (7 tests).

**Missing Edge Cases**:
- ❌ Suicide with pending storage changes (should they revert?)
- ❌ Double suicide (suicide in parent and child)
- ❌ Suicide resurrection (create new contract at same address after suicide)

### 6. System Call Edge Cases

**Current State**: Good coverage (8 tests).

**Missing Edge Cases**:
- ❌ Failed system call (system call reverts)
- ❌ System call with maximum gas usage
- ❌ Multiple EIP system calls in one block (EIP-4788 + EIP-7002 + EIP-7251)

### 7. Weird States & Rare Scenarios

**Untested Weird States**:
- ❌ Empty block (no transactions, no system calls) - **TESTED** ✅ (in system_call_no_transactions)
- ❌ Block with only reverted transactions
- ❌ Block with all transactions out of gas
- ❌ Extremely deep nesting (1000+ calls)
- ❌ Transaction with no gas (intrinsic gas exactly equals gas limit)
- ❌ Call stack unwinding during panic
- ❌ Reentrancy scenarios (A→B→A→B→A)
- ❌ Contract calling itself recursively

### 8. Error Conditions

**Untested Error Scenarios**:
- ❌ OnBlockEnd with error
- ❌ OnTxEnd with wrapped errors
- ❌ Invalid call depth
- ❌ Invalid opcode
- ❌ Invalid balance change reason (should panic)
- ❌ Invalid gas change reason (should panic)

## Priority Matrix

### P0 (CRITICAL - Block Production) ✅ COMPLETED
1. ✅ **Block-level rewards** (miner, uncle, transaction fees) - 5 tests in `tracer_block_level_test.go`
2. ✅ **Withdrawals** (EIP-4895) - 4 tests in `tracer_block_level_test.go`
3. ✅ **Block-level code changes** - 3 tests in `tracer_block_level_test.go`

### P1 (HIGH - Common Edge Cases)
4. Maximum call depth
5. Precompiled contracts (all of them)
6. Out of gas scenarios
7. Large data scenarios (input/output/logs)

### P2 (MEDIUM - Rare but Important)
8. OnSkippedBlock
9. CREATE2 collisions
10. STATICCALL violations
11. Reentrancy patterns

### P3 (LOW - Extreme Edge Cases)
12. OnOpcodeFault
13. OnKeccakPreimage
14. Balance overflow scenarios
15. Storage collisions

## Recommendations

### Immediate Actions ✅ COMPLETED
1. ✅ Add block-level balance change tests (rewards) - **5 tests added**
2. ✅ Add block-level code change tests - **3 tests added**
3. ✅ Add withdrawal tests - **4 tests added**
4. ✅ Add complex block scenarios - **2 tests added**
5. ✅ Document remaining gaps - **This file serves as documentation**

### Follow-up Actions
1. Add maximum call depth test
2. Add comprehensive precompile tests
3. Add out of gas edge cases
4. Add reentrancy tests
