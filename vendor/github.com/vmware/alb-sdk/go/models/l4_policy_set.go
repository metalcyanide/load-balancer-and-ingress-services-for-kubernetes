// Copyright 2021 VMware, Inc.
// SPDX-License-Identifier: Apache License 2.0
package models

// This file is auto-generated.

// L4PolicySet l4 policy set
// swagger:model L4PolicySet
type L4PolicySet struct {

	// UNIX time since epoch in microseconds. Units(MICROSECONDS).
	// Read Only: true
	LastModified *string `json:"_last_modified,omitempty"`

	// Creator name. Field introduced in 17.2.7.
	CreatedBy *string `json:"created_by,omitempty"`

	//  Field introduced in 17.2.7.
	Description *string `json:"description,omitempty"`

	//  Field introduced in 17.2.7.
	IsInternalPolicy *bool `json:"is_internal_policy,omitempty"`

	// Policy to apply when a new transport connection is setup. Field introduced in 17.2.7.
	L4ConnectionPolicy *L4ConnectionPolicy `json:"l4_connection_policy,omitempty"`

	// Key value pairs for granular object access control. Also allows for classification and tagging of similar objects. Field deprecated in 20.1.5. Field introduced in 20.1.2. Maximum of 4 items allowed.
	Labels []*KeyValue `json:"labels,omitempty"`

	// List of labels to be used for granular RBAC. Field introduced in 20.1.5. Allowed in Basic edition, Essentials edition, Enterprise edition.
	Markers []*RoleFilterMatchLabel `json:"markers,omitempty"`

	// Name of the L4 Policy Set. Field introduced in 17.2.7.
	// Required: true
	Name *string `json:"name"`

	//  It is a reference to an object of type Tenant. Field introduced in 17.2.7.
	TenantRef *string `json:"tenant_ref,omitempty"`

	// url
	// Read Only: true
	URL *string `json:"url,omitempty"`

	// ID of the L4 Policy Set. Field introduced in 17.2.7.
	UUID *string `json:"uuid,omitempty"`
}
