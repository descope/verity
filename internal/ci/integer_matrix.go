package ci

const IntegerMatrixShardSize = 64

func IntegerMatrixShard(index int) int {
	return index/IntegerMatrixShardSize + 1
}
