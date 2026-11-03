package project

import "time"

type Role string

const (
	RoleOwner  Role = "owner"
	RoleEditor Role = "editor"
	RoleViewer Role = "viewer"
)

type Project struct {
	ID             string
	Name           string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Role           Role
	OutputLanguage string
}

type Member struct {
	ProjectID string
	Subject   string
	Role      Role
}

func (r Role) CanRead() bool {
	return r == RoleOwner || r == RoleEditor || r == RoleViewer
}

func (r Role) CanEdit() bool {
	return r == RoleOwner || r == RoleEditor
}

func (r Role) CanManageMembers() bool {
	return r == RoleOwner
}

func (r Role) Valid() bool {
	return r.CanRead()
}
