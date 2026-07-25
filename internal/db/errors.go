package db

import "errors"

var ErrNotFound = errors.New("not found")
var ErrInviteUnavailable = errors.New("invite unavailable")
var ErrRegistrationTransition = errors.New("registration state transition rejected")
var ErrTemplateInUse = errors.New("template in use")
var ErrTemplateIsDefault = errors.New("template is default")
