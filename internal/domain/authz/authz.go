package authz

import (
	"errors"
	"slices"
	"strings"
)

type Capability string

const (
	CapabilityAdminAccess            Capability = "admin.access"
	CapabilityContentCreate          Capability = "content.create"
	CapabilityContentReadPrivate     Capability = "content.read_private"
	CapabilityContentEdit            Capability = "content.edit"
	CapabilityContentEditOwn         Capability = "content.edit_own"
	CapabilityContentEditOthers      Capability = "content.edit_others"
	CapabilityContentPublish         Capability = "content.publish"
	CapabilityContentSchedule        Capability = "content.schedule"
	CapabilityContentArchive         Capability = "content.archive"
	CapabilityContentDelete          Capability = "content.delete"
	CapabilityContentRestore         Capability = "content.restore"
	CapabilityContentManageRevisions Capability = "content.manage_revisions"
	CapabilityMediaUpload            Capability = "media.upload"
	CapabilityMediaEdit              Capability = "media.edit"
	CapabilityMediaDelete            Capability = "media.delete"
	CapabilityMediaReadPrivate       Capability = "media.read_private"
	CapabilityTaxonomiesManage       Capability = "taxonomies.manage"
	CapabilityTaxonomiesAssign       Capability = "taxonomies.assign"
	CapabilityMenusManage            Capability = "menus.manage"
	CapabilitySettingsView           Capability = "settings.view"
	CapabilitySettingsManage         Capability = "settings.manage"
	CapabilityUsersView              Capability = "users.view"
	CapabilityUsersCreate            Capability = "users.create"
	CapabilityUsersEdit              Capability = "users.edit"
	CapabilityUsersDelete            Capability = "users.delete"
	CapabilityRolesView              Capability = "roles.view"
	CapabilityRolesManage            Capability = "roles.manage"
	CapabilityRESTAccess             Capability = "rest.access"
	CapabilityRESTAccessPrivate      Capability = "rest.access_private"
	CapabilityRESTWrite              Capability = "rest.write"
	CapabilityAuditView              Capability = "audit.view"
	CapabilityAuditExport            Capability = "audit.export"
)

var knownCapabilities = map[Capability]struct{}{
	CapabilityAdminAccess: {}, CapabilityContentCreate: {}, CapabilityContentReadPrivate: {},
	CapabilityContentEdit: {}, CapabilityContentEditOwn: {}, CapabilityContentEditOthers: {},
	CapabilityContentPublish: {}, CapabilityContentSchedule: {}, CapabilityContentArchive: {},
	CapabilityContentDelete: {}, CapabilityContentRestore: {}, CapabilityContentManageRevisions: {},
	CapabilityMediaUpload: {}, CapabilityMediaEdit: {}, CapabilityMediaDelete: {},
	CapabilityMediaReadPrivate: {}, CapabilityTaxonomiesManage: {}, CapabilityTaxonomiesAssign: {},
	CapabilityMenusManage: {}, CapabilitySettingsView: {}, CapabilitySettingsManage: {},
	CapabilityUsersView: {}, CapabilityUsersCreate: {}, CapabilityUsersEdit: {},
	CapabilityUsersDelete: {}, CapabilityRolesView: {}, CapabilityRolesManage: {},
	CapabilityRESTAccess: {}, CapabilityRESTAccessPrivate: {}, CapabilityRESTWrite: {},
	CapabilityAuditView: {}, CapabilityAuditExport: {},
}

type Role struct {
	ID           string       `json:"id"`
	Label        string       `json:"label"`
	Capabilities []Capability `json:"capabilities"`
}

type Principal struct {
	ID           string
	Capabilities map[Capability]struct{}
	Anonymous    bool
}

func Anonymous() Principal {
	return Principal{ID: "anonymous", Anonymous: true}
}

func NewPrincipal(id string, capabilities ...Capability) Principal {
	resolved := make(map[Capability]struct{}, len(capabilities))
	for _, capability := range capabilities {
		resolved[capability] = struct{}{}
	}
	return Principal{ID: id, Capabilities: resolved}
}

func (principal Principal) Has(capability Capability) bool {
	_, allowed := principal.Capabilities[capability]
	return allowed
}

func (principal Principal) CanEdit(authorID string) bool {
	switch {
	case principal.Has(CapabilityContentEdit), principal.Has(CapabilityContentEditOthers):
		return true
	case principal.Has(CapabilityContentEditOwn):
		return principal.ID != "" && principal.ID == authorID
	default:
		return false
	}
}

func (capability Capability) Valid() bool {
	_, valid := knownCapabilities[capability]
	return valid
}

func (role Role) Validate() error {
	if strings.TrimSpace(role.ID) == "" || strings.TrimSpace(role.Label) == "" {
		return errors.New("role id and label are required")
	}
	seen := make(map[Capability]struct{}, len(role.Capabilities))
	for _, capability := range role.Capabilities {
		if !capability.Valid() {
			return errors.New("role capability is invalid")
		}
		if _, duplicated := seen[capability]; duplicated {
			return errors.New("role capability is duplicated")
		}
		seen[capability] = struct{}{}
	}
	return nil
}

func (role Role) Principal(id string) Principal {
	return NewPrincipal(id, role.Capabilities...)
}

func AdministratorRole() Role {
	capabilities := make([]Capability, 0, len(knownCapabilities))
	for capability := range knownCapabilities {
		capabilities = append(capabilities, capability)
	}
	slices.Sort(capabilities)
	return Role{ID: "administrator", Label: "Administrator", Capabilities: capabilities}
}

func EditorRole() Role {
	return Role{
		ID: "editor", Label: "Editor",
		Capabilities: []Capability{
			CapabilityAdminAccess, CapabilityContentCreate, CapabilityContentReadPrivate,
			CapabilityContentEditOwn, CapabilityContentPublish, CapabilityContentSchedule,
			CapabilityContentArchive, CapabilityContentManageRevisions,
			CapabilityMediaUpload, CapabilityMediaEdit, CapabilityTaxonomiesAssign,
			CapabilityRESTAccess, CapabilityRESTAccessPrivate, CapabilityRESTWrite,
		},
	}
}
