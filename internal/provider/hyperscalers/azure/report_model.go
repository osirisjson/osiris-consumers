// This file builds the format-neutral Azure report model: the
// document's own provenance (Signature), the management-group/
// subscription hierarchy, a resource-group inventory, and one tracked
// category per confirmed Azure resource type (icons.go's armTypeIcon
// set, expressed here as OSIRIS types) plus a generic fallback category
// for anything unrecognized. Unlike the topology page, this report is a
// complete inventory it does not hide storage.volume or a resource
// group's own container.resourcegroup entry the way the curated
// topology page does; the resource group's own entry is instead
// disclosed via its own summary row, not dropped.
//
// It is read-only over the generic graph, never re-parsing the
// document; see report_render.go for the Markdown emission and
// report_xlsx.go for the .xlsx companion.
package azure

import (
	"sort"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
)

// resourceGroupType and the two hierarchy types this report reads.
const (
	resourceGroupType   = "logical.resourcegroup"
	managementGroupType = "logical.managementgroup"
	subscriptionType    = "logical.subscription"
	rgResourceType      = "container.resourcegroup"
)

// categoryOrder maps a confirmed OSIRIS JSON resource type (icons.go's
// armTypeIcon set) to its report section label, in emission order.
// "container.resourcegroup" is deliberately absent it is disclosed
// through the resource-group summary table instead of a category row,
// never both and never neither. A type not listed here (including one
// this package has simply not seen in a confirmed document yet) falls
// back to the "Other resources" category, never dropped.
var categoryOrder = []struct{ osirisType, label string }{
	{"compute.vm", "Virtual Machines"},
	{"osiris.azure.storage", "Storage Accounts"},
	{"storage.volume", "Disks"},
	{"network.vpc", "Virtual Networks"},
	{"network.subnet", "Subnets"},
	{"network.security.group", "Network Security Groups"},
	{"osiris.azure.publicip", "Public IP Addresses"},
	{"network.loadbalancer", "Load Balancers"},
	{"network.interface", "Network Interfaces"},
	{"osiris.azure.bastion", "Bastion Hosts"},
	{"osiris.azure.sqlvm", "SQL Virtual Machines"},
}

const otherCategoryLabel = "Other resources"

// azureXLSXRowThreshold is the row count, per enumerable category, that
// triggers the .xlsx companion workbook, same used by other providers
// we set the default value to 50 raw after that the file is generated.
const azureXLSXRowThreshold = 50

// ResourceRow is one resource's disclosed inventory fields, common
// across every tracked category. ResourceGroup is resolved via its
// logical.resourcegroup membership, "(ungrouped)" when the document
// puts it in none. Tags is the resource's own "tags" map verbatim, a
// tag's key is user-defined, not a fixed Azure vocabulary, so it is
// never normalized (no case-folding, no trimming, no renaming): two
// resources spelling a tag differently really do have two different
// tags. A renderer pivots this into one column per distinct key it
// sees across the rows it is about to show, rather than one joined
// "key=value, ..." cell.
type ResourceRow struct {
	Name, ResourceID, ResourceGroup, Region, ARMType, Status, State string
	Tags                                                            map[string]string
}

// ResourceCategory is one tracked type's full row set.
type ResourceCategory struct {
	Label string
	Rows  []ResourceRow
}

// ResourceGroupRow is one logical.resourcegroup's own summary: its
// fields from the document's own container.resourcegroup resource entry
// when one exists (a document could in principle carry the group
// without it), plus its total member count. Tags is verbatim, same
// no-normalization rule as ResourceRow.Tags.
type ResourceGroupRow struct {
	Name, Region, Status, State string
	Tags                        map[string]string
	ResourceCount               int
}

// HierarchyRow is one management-group or subscription group, with its
// immediate parent's name ("(root)" when it has none) a flat table
// avoids needing tree rendering for what is, in every document seen so
// far, at most a handful of levels.
type HierarchyRow struct {
	Kind, Name, Parent string
}

// SignatureInfo is the document's own provenance: the exact input hash
// plus every "metadata" field the document carries. Every string is
// "(absent)" when the document did not carry it, never fabricated.
type SignatureInfo struct {
	InputSHA256                     string
	Timestamp                       string
	GeneratorName, GeneratorVersion string
	GeneratorURL                    string // empty, not "(absent)", when the render should omit the link
	ScopeName                       string // "" (not "(absent)") when absent the report title falls back to a plain title, never a fabricated name
	ScopePurpose                    string
	ScopeProviders                  []string
	ScopeAccounts                   []string
	ScopeRegions                    []string
	ScopeSubscriptions              []string
	ExtraMetadataKeys               []string
}

// ReportModel is the complete, format-neutral Azure report.
type ReportModel struct {
	Signature      SignatureInfo
	Hierarchy      []HierarchyRow
	ResourceGroups []ResourceGroupRow
	Categories     []ResourceCategory
	Other          ResourceCategory
	Regions        []string
}

// NeedsXLSXCompanion reports whether any tracked category (every
// confirmed category, "Other resources", or the resource-group summary
// itself) exceeds azureXLSXRowThreshold rows.
func (m ReportModel) NeedsXLSXCompanion() bool {
	if len(m.ResourceGroups) > azureXLSXRowThreshold || len(m.Other.Rows) > azureXLSXRowThreshold {
		return true
	}
	for _, c := range m.Categories {
		if len(c.Rows) > azureXLSXRowThreshold {
			return true
		}
	}
	return false
}

// BuildReportModel builds the report over g. md and inputSHA carry the
// document's own provenance into the Signature section.
func BuildReportModel(g *graph.Graph, md document.Metadata, inputSHA string) ReportModel {
	nodeToRG := resourceGroupMembership(g)
	rgNames := resourceGroupNames(g)

	labelFor := make(map[string]string, len(categoryOrder))
	order := make(map[string]int, len(categoryOrder))
	for i, c := range categoryOrder {
		labelFor[c.osirisType] = c.label
		order[c.osirisType] = i
	}
	rowsByType := make(map[string][]ResourceRow, len(categoryOrder))
	var other []ResourceRow
	regionSet := make(map[string]bool)

	for _, id := range g.ResourceIDs {
		res := g.Resources[id]
		if res == nil || res.Type == rgResourceType {
			continue
		}
		if res.Provider.Region != "" {
			regionSet[res.Provider.Region] = true
		}
		row := ResourceRow{
			Name:          nameOrID(res.Name, id),
			ResourceID:    id,
			ResourceGroup: rgDisplayName(nodeToRG, rgNames, id),
			Region:        stringOrAbsent(res.Provider.Region),
			ARMType:       stringOrAbsent(res.Provider.Type),
			Status:        stringOrAbsent(res.Status),
			State:         stringOrAbsent(res.State),
			Tags:          res.Tags,
		}
		if _, ok := labelFor[res.Type]; ok {
			rowsByType[res.Type] = append(rowsByType[res.Type], row)
			continue
		}
		other = append(other, row)
	}

	var categories []ResourceCategory
	for _, c := range categoryOrder {
		categories = append(categories, ResourceCategory{Label: c.label, Rows: rowsByType[c.osirisType]})
	}

	regions := make([]string, 0, len(regionSet))
	for r := range regionSet {
		regions = append(regions, r)
	}
	sort.Strings(regions)

	return ReportModel{
		Signature:      buildSignature(md, inputSHA),
		Hierarchy:      buildHierarchy(g),
		ResourceGroups: buildResourceGroups(g, nodeToRG),
		Categories:     categories,
		Other:          ResourceCategory{Label: otherCategoryLabel, Rows: other},
		Regions:        regions,
	}
}

// resourceGroupMembership maps a resource ID to the ID of the
// logical.resourcegroup group that lists it as a member the same
// "effective container" rule the topology page uses (layout.go's
// buildHierarchyIndex), independently derived here since the report
// works directly over *graph.Graph, never the topology model.
func resourceGroupMembership(g *graph.Graph) map[string]string {
	nodeToRG := make(map[string]string)
	for _, gid := range g.GroupIDs {
		grp := g.Groups[gid]
		if grp == nil || grp.Type != resourceGroupType {
			continue
		}
		for _, mid := range g.GroupMembers[gid] {
			if _, already := nodeToRG[mid]; !already {
				nodeToRG[mid] = gid
			}
		}
	}
	return nodeToRG
}

// resourceGroupNames maps a logical.resourcegroup group ID to its own
// display name.
func resourceGroupNames(g *graph.Graph) map[string]string {
	names := make(map[string]string)
	for _, gid := range g.GroupIDs {
		grp := g.Groups[gid]
		if grp == nil || grp.Type != resourceGroupType {
			continue
		}
		names[gid] = nameOrID(grp.Name, gid)
	}
	return names
}

// rgDisplayName resolves id's owning resource group's display name, or
// "(ungrouped)" when the document puts it in no resource group.
func rgDisplayName(nodeToRG, rgNames map[string]string, id string) string {
	rgID, ok := nodeToRG[id]
	if !ok {
		return "(ungrouped)"
	}
	if name, ok := rgNames[rgID]; ok {
		return name
	}
	return rgID
}

// buildHierarchy lists every management-group and subscription group,
// each with its immediate parent's own display name.
func buildHierarchy(g *graph.Graph) []HierarchyRow {
	var rows []HierarchyRow
	for _, gid := range g.GroupIDs {
		grp := g.Groups[gid]
		if grp == nil {
			continue
		}
		var kind string
		switch grp.Type {
		case managementGroupType:
			kind = "Management Group"
		case subscriptionType:
			kind = "Subscription"
		default:
			continue
		}
		parent := "(root)"
		for _, pid := range g.ParentOfGroups[gid] {
			if p := g.Groups[pid]; p != nil {
				parent = nameOrID(p.Name, pid)
				break
			}
		}
		rows = append(rows, HierarchyRow{Kind: kind, Name: nameOrID(grp.Name, gid), Parent: parent})
	}
	return rows
}

// buildResourceGroups lists every logical.resourcegroup group: its own
// container.resourcegroup resource's disclosed fields when the document
// carries one, plus its total member count (including that same self-
// entry, exactly as the document counts it).
func buildResourceGroups(g *graph.Graph, nodeToRG map[string]string) []ResourceGroupRow {
	memberCount := make(map[string]int)
	for id, rgID := range nodeToRG {
		_ = id
		memberCount[rgID]++
	}
	selfEntry := make(map[string]*document.Resource)
	for _, id := range g.ResourceIDs {
		res := g.Resources[id]
		if res != nil && res.Type == rgResourceType {
			if rgID, ok := nodeToRG[id]; ok {
				selfEntry[rgID] = res
			}
		}
	}

	var rows []ResourceGroupRow
	for _, gid := range g.GroupIDs {
		grp := g.Groups[gid]
		if grp == nil || grp.Type != resourceGroupType {
			continue
		}
		row := ResourceGroupRow{
			Name:          nameOrID(grp.Name, gid),
			ResourceCount: memberCount[gid],
			Region:        "(absent)",
			Status:        "(absent)",
			State:         "(absent)",
		}
		if res := selfEntry[gid]; res != nil {
			row.Region = stringOrAbsent(res.Provider.Region)
			row.Status = stringOrAbsent(res.Status)
			row.State = stringOrAbsent(res.State)
			row.Tags = res.Tags
		}
		rows = append(rows, row)
	}
	return rows
}

func buildSignature(md document.Metadata, inputSHA string) SignatureInfo {
	s := SignatureInfo{
		InputSHA256: inputSHA,
		Timestamp:   stringOrAbsent(md.Timestamp),
	}
	if md.Generator != nil {
		s.GeneratorName = stringOrAbsent(md.Generator.Name)
		s.GeneratorVersion = stringOrAbsent(md.Generator.Version)
		s.GeneratorURL = md.Generator.URL
	} else {
		s.GeneratorName = "(absent)"
		s.GeneratorVersion = "(absent)"
	}
	scope := decodeAzureScope(md.Scope)
	s.ScopeName = scope.Name
	s.ScopePurpose = stringOrAbsent(scope.Purpose)
	s.ScopeProviders = scope.Providers
	s.ScopeAccounts = scope.Accounts
	s.ScopeRegions = scope.Regions
	s.ScopeSubscriptions = scope.Subscriptions
	for k := range md.Extra {
		s.ExtraMetadataKeys = append(s.ExtraMetadataKeys, k)
	}
	sort.Strings(s.ExtraMetadataKeys)
	return s
}

// distinctTagKeys returns the sorted union of every key across tagMaps
// the column set a table pivots a set of rows' Tags maps into
// (report_render.go, report_xlsx.go). Sorted only for deterministic
// output; a key's own spelling (case, spacing, symbols) is never
// altered, since a tag key is user-defined, not a fixed Azure
// taxonomy.
func distinctTagKeys(tagMaps []map[string]string) []string {
	seen := make(map[string]bool)
	for _, tags := range tagMaps {
		for k := range tags {
			seen[k] = true
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// tagValueOrAbsent returns tags[key], or "(absent)" when tags carries
// no such key evidence-state, never a blank cell that could be
// mistaken for an asserted empty value.
func tagValueOrAbsent(tags map[string]string, key string) string {
	if v, ok := tags[key]; ok {
		return v
	}
	return "(absent)"
}

// nameOrID returns name, or id when the document gave no name
// mirrors topology.Node.Label's own fallback rule.
func nameOrID(name, id string) string {
	if name == "" {
		return id
	}
	return name
}
