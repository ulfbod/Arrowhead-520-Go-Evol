module arrowhead/core

go 1.26.0

require (
	arrowhead/authzforce v0.0.0
	google.golang.org/grpc v1.64.0
	google.golang.org/protobuf v1.34.2
)

require (
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240604185151-ef581f913117 // indirect
)

replace arrowhead/authzforce => ../shared/authzforce
