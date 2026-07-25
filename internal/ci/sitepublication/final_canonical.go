package sitepublication

var finalPlanCodec = canonicalJSONCodec[FinalPlan]{
	label: "final publication plan", invalid: ErrInvalidFinalPlan, validate: validateFinalPlan,
}

func MarshalFinalPlanCanonical(plan *FinalPlan) ([]byte, error) {
	return finalPlanCodec.marshal(plan)
}

func ParseFinalPlanCanonical(data []byte) (FinalPlan, error) {
	return finalPlanCodec.parse(data)
}
