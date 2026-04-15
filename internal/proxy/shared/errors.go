package shared

type APIError struct {
	Status int
	Type string
	Message string
}
