package networking

import "testing"

func TestAnalyzeReachability_DefaultsDeny(t *testing.T) {
	res := AnalyzeReachability(ReachabilityRequest{
		SourceType: "instance", SourceID: "i1",
		DestinationType: "instance", DestinationID: "i2",
		Protocol: "tcp", Port: 22,
	})
	if res.Allowed {
		t.Fatal("expected deny when SG data missing")
	}
}

func TestAnalyzeReachabilityWithVPC_NoSGsDeny(t *testing.T) {
	res := AnalyzeReachabilityWithVPC(ReachabilityRequest{
		SourceType: "instance", SourceID: "i1",
		DestinationType: "instance", DestinationID: "i2",
		Protocol: "tcp", Port: 22,
	}, nil, nil, 22)
	if res.Allowed {
		t.Fatal("expected deny with empty security groups")
	}
}
