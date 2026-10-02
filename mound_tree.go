package pq

type MoundTree interface {
	Insert(CDNData)
	ExtractMin() CDNData
	RelaxExtractMin() CDNData

	binarySearch(leaf, v uint32) uint32
}

var (
	_ MoundTree = (*NCASMoundTree)(nil)
	_ MoundTree = (*MCASMoundTree)(nil)
)
