package event

type UserChangedDetails struct {
	UserId string  `json:"userId"`
	Bio    *string `json:"bio,omitempty"`
	Image  *string `json:"image,omitempty"`
}

func (e UserChangedDetails) Tags() []string {
	return []string{UserIdTagPrefix(e.UserId)}
}
