package graph

import (
	"context"
	"fmt"

	"github.com/microsoftgraph/msgraph-sdk-go/groups"
	msgraphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// ADUser represents a user from Azure Active Directory.
type ADUser struct {
	ID           string `json:"id"`
	DisplayName  string `json:"display_name"`
	Mail         string `json:"mail"`
	JobTitle     string `json:"job_title,omitempty"`
	Department   string `json:"department,omitempty"`
	CompanyName  string `json:"company_name,omitempty"`
	UserType     string `json:"user_type,omitempty"` // "Member" or "Guest"
	ManagerEmail string `json:"manager_email,omitempty"`
}

// ADGroup represents a security group from Azure Active Directory.
type ADGroup struct {
	ID           string   `json:"id"`
	DisplayName  string   `json:"display_name"`
	Description  string   `json:"description,omitempty"`
	MemberEmails []string `json:"member_emails,omitempty"`
}

// ListUsers lists all users in the tenant with relevant profile fields.
// Uses the /users endpoint with delegated auth (requires User.Read.All scope).
func (c *GraphClient) ListUsers(ctx context.Context) ([]ADUser, error) {
	var allUsers []ADUser
	top := int32(100)

	selectFields := []string{"id", "displayName", "mail", "jobTitle", "department", "companyName", "userType"}
	config := &users.UsersRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.UsersRequestBuilderGetQueryParameters{
			Select: selectFields,
			Top:    &top,
		},
	}

	result, err := c.graphClient.Users().Get(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("graph: listing users: %w", err)
	}

	allUsers = append(allUsers, convertUsers(result.GetValue())...)

	// Handle pagination
	for {
		nextLink := result.GetOdataNextLink()
		if nextLink == nil || *nextLink == "" {
			break
		}
		result, err = c.graphClient.Users().Get(ctx, config)
		if err != nil {
			return nil, fmt.Errorf("graph: listing users (pagination): %w", err)
		}
		allUsers = append(allUsers, convertUsers(result.GetValue())...)
	}

	return allUsers, nil
}

// GetUserManager returns the manager's email for a given user ID.
// Returns empty string if the user has no manager set.
func (c *GraphClient) GetUserManager(ctx context.Context, userID string) (string, error) {
	result, err := c.graphClient.Users().ByUserId(userID).Manager().Get(ctx, nil)
	if err != nil {
		// Manager not set is a common case, not an error
		return "", nil
	}

	if result == nil {
		return "", nil
	}

	// The manager response is a directoryObject; extract mail from additional data
	additionalData := result.GetAdditionalData()
	if mail, ok := additionalData["mail"]; ok {
		if mailStr, ok := mail.(*string); ok && mailStr != nil {
			return *mailStr, nil
		}
	}

	return "", nil
}

// ListGroups lists security groups in the tenant.
// Uses the /groups endpoint with delegated auth (requires Group.Read.All scope).
func (c *GraphClient) ListGroups(ctx context.Context) ([]ADGroup, error) {
	var allGroups []ADGroup
	top := int32(100)
	filter := "securityEnabled eq true"

	selectFields := []string{"id", "displayName", "description", "securityEnabled"}
	config := &groups.GroupsRequestBuilderGetRequestConfiguration{
		QueryParameters: &groups.GroupsRequestBuilderGetQueryParameters{
			Filter: &filter,
			Select: selectFields,
			Top:    &top,
		},
	}

	result, err := c.graphClient.Groups().Get(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("graph: listing groups: %w", err)
	}

	allGroups = append(allGroups, convertGroups(result.GetValue())...)

	// Handle pagination
	for {
		nextLink := result.GetOdataNextLink()
		if nextLink == nil || *nextLink == "" {
			break
		}
		result, err = c.graphClient.Groups().Get(ctx, config)
		if err != nil {
			return nil, fmt.Errorf("graph: listing groups (pagination): %w", err)
		}
		allGroups = append(allGroups, convertGroups(result.GetValue())...)
	}

	return allGroups, nil
}

// GetGroupMembers returns the email addresses of all members of a group.
func (c *GraphClient) GetGroupMembers(ctx context.Context, groupID string) ([]string, error) {
	result, err := c.graphClient.Groups().ByGroupId(groupID).Members().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("graph: listing members for group %s: %w", groupID, err)
	}

	var emails []string
	for _, member := range result.GetValue() {
		additionalData := member.GetAdditionalData()
		if mail, ok := additionalData["mail"]; ok {
			if mailStr, ok := mail.(*string); ok && mailStr != nil && *mailStr != "" {
				emails = append(emails, *mailStr)
			}
		}
	}

	return emails, nil
}

// convertUsers converts Graph SDK user models to ADUser structs.
func convertUsers(items []msgraphmodels.Userable) []ADUser {
	result := make([]ADUser, 0, len(items))
	for _, u := range items {
		user := ADUser{}
		if id := u.GetId(); id != nil {
			user.ID = *id
		}
		if name := u.GetDisplayName(); name != nil {
			user.DisplayName = *name
		}
		if mail := u.GetMail(); mail != nil {
			user.Mail = *mail
		}
		if title := u.GetJobTitle(); title != nil {
			user.JobTitle = *title
		}
		if dept := u.GetDepartment(); dept != nil {
			user.Department = *dept
		}
		if company := u.GetCompanyName(); company != nil {
			user.CompanyName = *company
		}
		if userType := u.GetUserType(); userType != nil {
			user.UserType = *userType
		}
		// Skip users without email (service accounts, etc.)
		if user.Mail == "" {
			continue
		}
		result = append(result, user)
	}
	return result
}

// convertGroups converts Graph SDK group models to ADGroup structs.
func convertGroups(items []msgraphmodels.Groupable) []ADGroup {
	result := make([]ADGroup, 0, len(items))
	for _, g := range items {
		group := ADGroup{}
		if id := g.GetId(); id != nil {
			group.ID = *id
		}
		if name := g.GetDisplayName(); name != nil {
			group.DisplayName = *name
		}
		if desc := g.GetDescription(); desc != nil {
			group.Description = *desc
		}
		result = append(result, group)
	}
	return result
}
