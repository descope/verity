package sitepublication

var signerPlanCodec = canonicalJSONCodec[SignerPlan]{
	label: "signer plan", invalid: ErrInvalidSignerPlan, validate: ValidateSignerPlan,
}

func MarshalSignerPlanCanonical(plan *SignerPlan) ([]byte, error) {
	return signerPlanCodec.marshal(plan)
}

func ParseSignerPlanCanonical(data []byte) (SignerPlan, error) {
	return signerPlanCodec.parse(data)
}
