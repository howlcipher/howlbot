# Change log

## Mission007 remediation

F8: authorize before pending reservation; commit only after policy ALLOW; cancellation preserves retries on denial/evaluator failure; concurrent handler history is protected. Added original-failure regression and race-tested handler coverage.
