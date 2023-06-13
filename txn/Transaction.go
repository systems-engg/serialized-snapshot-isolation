package txn

import (
	"serialized-snapshot-isolation/mvcc"
	"serialized-snapshot-isolation/txn/errors"
)

type ReadonlyTransaction struct {
	beginTimestamp uint64
	memtable       *mvcc.MemTable
	oracle         *Oracle
}

type ReadWriteTransaction struct {
	beginTimestamp uint64
	batch          *Batch
	reads          [][]byte
	memtable       *mvcc.MemTable
	oracle         *Oracle
}

func NewReadonlyTransaction(oracle *Oracle) *ReadonlyTransaction {
	return &ReadonlyTransaction{
		beginTimestamp: oracle.beginTimestamp(),
		oracle:         oracle,
		memtable:       oracle.transactionExecutor.memtable,
	}
}

func NewReadWriteTransaction(oracle *Oracle) *ReadWriteTransaction {
	return &ReadWriteTransaction{
		beginTimestamp: oracle.beginTimestamp(),
		batch:          NewBatch(),
		oracle:         oracle,
		memtable:       oracle.transactionExecutor.memtable,
	}
}

func (transaction *ReadonlyTransaction) Get(key []byte) (mvcc.Value, bool) {
	versionedKey := mvcc.NewVersionedKey(key, transaction.beginTimestamp)
	return transaction.memtable.Get(versionedKey)
}

func (transaction *ReadonlyTransaction) Finish() {
	transaction.oracle.finishBeginTimestampForReadonlyTransaction(transaction)
}

func (transaction *ReadWriteTransaction) Get(key []byte) (mvcc.Value, bool) {
	if value, ok := transaction.batch.Get(key); ok {
		return mvcc.NewValue(value), true
	}
	transaction.reads = append(transaction.reads, key)

	versionedKey := mvcc.NewVersionedKey(key, transaction.beginTimestamp)
	return transaction.memtable.Get(versionedKey)
}

func (transaction *ReadWriteTransaction) PutOrUpdate(key []byte, value []byte) error {
	err := transaction.batch.Add(key, value)
	if err != nil {
		return err
	}
	return nil
}

func (transaction *ReadWriteTransaction) Commit() (<-chan struct{}, error) {
	if transaction.batch.IsEmpty() {
		return nil, errors.EmptyTransactionErr
	}

	//Send the transaction to the executor in the increasing order of commit timestamp
	transaction.oracle.executorLock.Lock()
	defer transaction.oracle.executorLock.Unlock()

	commitTimestamp, err := transaction.oracle.mayBeCommitTimestampFor(transaction)
	if err != nil {
		return nil, err
	}
	return transaction.oracle.transactionExecutor.Submit(transaction.batch.ToTimestampedBatch(commitTimestamp)), nil
}

func (transaction *ReadWriteTransaction) Finish() {
	transaction.oracle.finishBeginTimestampForReadWriteTransaction(transaction)
}
