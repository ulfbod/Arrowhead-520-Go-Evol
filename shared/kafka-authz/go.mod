module arrowhead/kafka-authz

go 1.26.0

require (
	arrowhead/authzforce v0.0.0
	github.com/segmentio/kafka-go v0.4.47
)

require (
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/pierrec/lz4/v4 v4.1.18 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)

replace arrowhead/authzforce => ../authzforce
