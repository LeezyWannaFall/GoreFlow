package application

import "errors"

var ErrJobNotFound = errors.New("job not found")
var ErrNoJobAvailable = errors.New("no job available")
