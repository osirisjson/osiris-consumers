package azure

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.osirisjson.org/consumers/internal/document"
	"go.osirisjson.org/consumers/internal/graph"
	"go.osirisjson.org/consumers/internal/topology"
)

var fixedTime = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

func loadFixture(t *testing.T, rel string) (*document.Document, *graph.Graph, *topology.Topology) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", rel, err)
	}
	doc, err := document.Parse(raw)
	if err != nil {
		t.Fatalf("parsing fixture %s: %v", rel, err)
	}
	g, err := graph.Build(context.Background(), doc)
	if err != nil {
		t.Fatalf("indexing fixture %s: %v", rel, err)
	}
	m, err := topology.Build(context.Background(), doc, g)
	if err != nil {
		t.Fatalf("building topology model for %s: %v", rel, err)
	}
	return doc, g, m
}

const azureFixture = "producer/hyperscaler/azure/azure.json"

func renderXML(t *testing.T, doc *document.Document, g *graph.Graph, m *topology.Topology, artifactTime time.Time) string {
	t.Helper()
	var buf bytes.Buffer
	r := NewTopologyRenderer(m, g, doc.Metadata, artifactTime)
	if err := r.Render(context.Background(), &buf); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return buf.String()
}

func TestApplies(t *testing.T) {
	_, g, _ := loadFixture(t, azureFixture)
	if !Applies(g) {
		t.Fatal("expected Azure fixture to trigger Applies")
	}
}

func TestRenderWellFormedAndFramed(t *testing.T) {
	doc, g, m := loadFixture(t, azureFixture)
	out := renderXML(t, doc, g, m, fixedTime)
	if !strings.HasPrefix(out, "<mxfile ") || !strings.HasSuffix(out, "</mxfile>\n") {
		t.Errorf("output not framed as an mxfile")
	}
	if !strings.Contains(out, `<diagram id="osirisjson-azure-topology"`) {
		t.Errorf("fixed diagram id missing")
	}
	var v any
	if err := xml.Unmarshal([]byte(out), &v); err != nil {
		t.Errorf("output is not well-formed XML: %v", err)
	}
}

func TestRenderIsByteIdentical(t *testing.T) {
	doc, g, m := loadFixture(t, azureFixture)
	if renderXML(t, doc, g, m, fixedTime) != renderXML(t, doc, g, m, fixedTime) {
		t.Fatal("two renders of the same model differ")
	}
}

func TestTitleShowsScopeNameAndGenericCounts(t *testing.T) {
	doc, g, m := loadFixture(t, azureFixture)
	out := renderXML(t, doc, g, m, fixedTime)
	c := m.Counts()
	if !strings.Contains(out, "00000000-0000-0000-0000-000000000000 - FACTORY-00-sub") {
		t.Errorf("title does not show metadata.scope.name")
	}
	if !strings.Contains(out, "13 nodes, 7 edges, 4 groups") {
		t.Errorf("title does not show the generic model's exact counts (got model counts %+v)", c)
	}
}

func TestFooterShowsDocumentMetadata(t *testing.T) {
	doc, g, m := loadFixture(t, azureFixture)
	out := renderXML(t, doc, g, m, fixedTime)
	for _, want := range []string{
		"Snapshot created on", "2026-09-16T00:00:00Z", // metadata.timestamp
		"Snapshot scope", "documentation", // metadata.scope.purpose
		"OSIRIS JSON for Microsoft Azure", "0.5.2", // metadata.generator.version
		"This topology was created with OSIRIS JSON",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("footer missing expected text %q", want)
		}
	}
}

func TestRenderKnownARMTypesGetAzureIcons(t *testing.T) {
	doc, g, m := loadFixture(t, azureFixture)
	out := renderXML(t, doc, g, m, fixedTime)
	for _, want := range []string{
		"img/lib/azure2/compute/Virtual_Machine.svg",
		"img/lib/azure2/networking/Virtual_Networks.svg",
		"img/lib/azure2/networking/Subnet.svg",
		"img/lib/azure2/networking/Network_Security_Groups.svg",
		"img/lib/azure2/networking/Public_IP_Addresses.svg",
		"img/lib/azure2/networking/Load_Balancers.svg",
		"img/lib/azure2/networking/Bastions.svg",
		"img/lib/azure2/networking/Network_Interfaces.svg",
		"img/lib/azure2/storage/Storage_Accounts.svg",
		"img/lib/azure2/databases/Azure_SQL_VM.svg",
		"img/lib/azure2/general/Subscriptions.svg",
		"img/lib/azure2/general/Management_Groups.svg",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected icon path %q not found in rendered output", want)
		}
	}
	if strings.Contains(out, "image=data:image/svg+xml,") == false {
		t.Errorf("expected the embedded OSIRIS JSON logo badge data URI to still be present")
	}
}

func TestResourceGroupContainerHasNoHeaderIconAndIsRightAligned(t *testing.T) {
	doc, g, m := loadFixture(t, azureFixture)
	out := renderXML(t, doc, g, m, fixedTime)
	if !strings.Contains(out, "align=right;fontSize=10;strokeColor=#6c8ebf;spacingRight=5;") {
		t.Errorf("resource-group container style missing right-aligned title")
	}
	if _, ok := iconForGroupType("logical.resourcegroup"); ok {
		t.Errorf("logical.resourcegroup must not have a header icon ")
	}
}

func TestHiddenTypesNeverAppearAsMemberIcons(t *testing.T) {
	doc, g, m := loadFixture(t, azureFixture)
	out := renderXML(t, doc, g, m, fixedTime)
	if strings.Contains(out, "img/lib/azure2/compute/Disks.svg") {
		t.Errorf("storage.volume must not be drawn on this page")
	}
}

func TestEmptyResourceGroupsRenderAsTitleOnlyBoxes(t *testing.T) {
	doc, g, m := loadFixture(t, azureFixture)
	out := renderXML(t, doc, g, m, fixedTime)
	if !strings.Contains(out, `height="78"`) {
		t.Errorf("expected at least one empty-resource-group box at the fixed collapsed height")
	}
}

// TestArrowsReflectDirectionField pins arrowsForDirection's full
// mapping directly independent of any fixture, since this is pure
// function behavior.
func TestArrowsReflectDirectionField(t *testing.T) {
	r := &TopologyRenderer{nodeType: map[string]string{}}
	for _, tt := range []struct {
		direction          string
		wantStart, wantEnd string
	}{
		{"forward", "none", "classic"},
		{"reverse", "classic", "none"},
		{"bidirectional", "classic", "classic"},
		{"", "none", "none"},
		{"some-future-value", "none", "none"},
	} {
		style, _ := r.edgeStyleAndLabel(topology.Edge{Type: "some-type", Direction: tt.direction})
		want := "endArrow=" + tt.wantEnd + ";startArrow=" + tt.wantStart + ";"
		if !strings.Contains(style, want) {
			t.Errorf("direction %q: style = %q, want it to contain %q", tt.direction, style, want)
		}
		if strings.Contains(style, "Arrow=block") {
			t.Errorf("direction %q: style = %q must never carry a block arrowhead", tt.direction, style)
		}
	}
}

func TestEdgeClassificationLabelsAndStyles(t *testing.T) {
	doc, g, m := loadFixture(t, azureFixture)
	out := renderXML(t, doc, g, m, fixedTime)
	// The arrowhead portion (endArrow/startArrow) now depends on each
	// edge's own real "direction" field (TestArrowsReflectDirectionField
	// covers that mapping directly), so this only pins the
	// direction-independent rest of each classification's style: label,
	// stroke color/width, and dash pattern.
	for _, want := range []struct{ label, styleSuffix string }{
		{"subnet attachment", "strokeColor=#6c8ebf;strokeWidth=1.7;fontSize=8;fontColor=#526b8e;labelBackgroundColor=#ffffff;dashed=1;dashPattern=3 4;"},
		{"subnet attachment", "strokeColor=#666666;strokeWidth=1.6;fontSize=8;fontColor=#444444;labelBackgroundColor=#ffffff;"},
		{"public IP association", ""},
		{"frontend IP", ""},
	} {
		if !strings.Contains(out, `value="`+want.label+`"`) {
			t.Errorf("expected edge labeled %q not found", want.label)
		}
		if want.styleSuffix != "" && !strings.Contains(out, want.styleSuffix) {
			t.Errorf("expected edge style suffix %q not found", want.styleSuffix)
		}
	}
	if strings.Contains(out, `value="contains"`) {
		t.Errorf("a nested subnet's own \"contains\" edge to its VNet must be consumed by the nesting, not also drawn as a line: %s", out)
	}
}

// TestEdgeClassifyContainsStyleIsStillDefined guards the "contains"
// entry in edgeClassify directly: layout.go always consumes a
// subnet-to-VNet "contains" edge once the subnet nests inside the
// VNet's own container (see TestEdgeClassificationLabelsAndStyles), so
// no fixture render ever exercises this classification's style/label
// text. A future relationship this package projects some other way
// (never nested, drawn as a line instead) would still need this exact
// mapping, so it is pinned here at the function level instead.
func TestEdgeClassifyContainsStyleIsStillDefined(t *testing.T) {
	r := &TopologyRenderer{nodeType: map[string]string{"s": "network.subnet", "v": "network.vpc"}}
	style, label := r.edgeStyleAndLabel(topology.Edge{Type: "contains", Source: "s", Target: "v"})
	if label != "contains" {
		t.Errorf("label = %q, want \"contains\"", label)
	}
	want := "rounded=0;html=1;endArrow=none;startArrow=none;strokeColor=#6c8ebf;strokeWidth=1.4;fontSize=8;fontColor=#526b8e;labelBackgroundColor=#ffffff;dashed=1;dashPattern=2 4;"
	if style != want {
		t.Errorf("style = %q, want %q", style, want)
	}
}

func TestEdgeClassifyFallsBackToLiteralConnectionTypeWhenUnconfirmed(t *testing.T) {
	style, label := (&TopologyRenderer{nodeType: map[string]string{}}).edgeStyleAndLabel(topology.Edge{Type: "some-future-type"})
	if label != "some-future-type" {
		t.Errorf("unclassified edge label = %q, want the literal connection type", label)
	}
	if !strings.Contains(style, "strokeWidth=1;") {
		t.Errorf("unclassified edge style = %q, want strokeWidth=1", style)
	}
	if strings.Contains(style, "Arrow=block") {
		t.Errorf("unclassified edge must never carry a block arrowhead")
	}
}

// TestEdgeTypeClassifyBGPAndPeeringStyles pins the bare-type (no
// source/target qualifier) styles for network.bgp and network.peering
// checked before edgeStyleAndLabel's generic fallback.
func TestEdgeTypeClassifyBGPAndPeeringStyles(t *testing.T) {
	r := &TopologyRenderer{nodeType: map[string]string{}}
	for _, tt := range []struct{ connType, want string }{
		{"network.bgp", "rounded=0;html=1;endArrow=none;startArrow=none;strokeColor=#3333FF;strokeWidth=2;fontSize=8;fontColor=#666666;labelBackgroundColor=#ffffff;dashed=1;"},
		{"network.peering", "rounded=0;html=1;endArrow=none;startArrow=none;strokeColor=#FF3399;strokeWidth=2;fontSize=8;fontColor=#666666;labelBackgroundColor=#ffffff;dashed=1;"},
	} {
		style, label := r.edgeStyleAndLabel(topology.Edge{Type: tt.connType})
		if label != tt.connType {
			t.Errorf("%s: label = %q, want %q", tt.connType, label, tt.connType)
		}
		if style != tt.want {
			t.Errorf("%s: style = %q, want %q", tt.connType, style, tt.want)
		}
	}
}

func TestIconPathForPrefersARMTypeOverrideOverOSIRISTypeDefault(t *testing.T) {
	r := &TopologyRenderer{
		armType: map[string]string{
			"n1": "Microsoft.Compute/virtualMachineScaleSets",
			"n2": "Microsoft.ContainerService/managedClusters", // hypothetical AKS ARM type, unmapped on purpose
		},
		nodeType: map[string]string{"n1": "compute.cluster", "n2": "compute.cluster"},
	}
	if path, ok := r.iconPathFor(topology.Node{ID: "n1", Type: "compute.cluster"}); !ok || path != "compute/VM_Scale_Sets.svg" {
		t.Errorf("VMSS-backed compute.cluster = (%q,%v), want (compute/VM_Scale_Sets.svg,true)", path, ok)
	}
	if path, ok := r.iconPathFor(topology.Node{ID: "n2", Type: "compute.cluster"}); !ok || path != "compute/Kubernetes_Services.svg" {
		t.Errorf("generic compute.cluster = (%q,%v), want (compute/Kubernetes_Services.svg,true) via the OSIRIS-type fallback", path, ok)
	}
}

func TestPropertyLabelLines(t *testing.T) {
	tests := []struct {
		osirisType string
		props      string
		want       string
	}{
		{"network.interface", `{"private_ip":"192.0.2.4"}`, "<br>192.0.2.4"},
		{"network.interface", `{}`, ""},
		{"network.interface", ``, ""},
		{"osiris.azure.publicip", `{"primary":true,"ip_address":"203.0.113.4","sku":"Standard"}`, "<br>true<br>203.0.113.4<br>Standard"},
		{"osiris.azure.publicip", `{"ip_address":"203.0.113.4"}`, "<br>203.0.113.4"},
		{
			"osiris.azure.aks.nodepool",
			`{"cluster_name":"aks1","orchestrator_version":"1.29","os_sku":"Ubuntu","os_type":"Linux","vm_size":"Standard_D2s_v3"}`,
			"<br>aks1<br>1.29<br>Ubuntu<br>Linux<br>Standard_D2s_v3",
		},
		{"osiris.azure.aks.nodepool", `{"cluster_name":"aks1"}`, "<br>aks1"},
		{"compute.vm", `{"private_ip":"192.0.2.4"}`, ""},
	}
	for _, tt := range tests {
		got := propertyLabelLines(tt.osirisType, json.RawMessage(tt.props))
		if got != tt.want {
			t.Errorf("propertyLabelLines(%q, %q) = %q, want %q", tt.osirisType, tt.props, got, tt.want)
		}
	}
}

func TestPropertyLabelLinesEscapesHTML(t *testing.T) {
	got := propertyLabelLines("network.interface", json.RawMessage(`{"private_ip":"<script>alert(1)</script>"}`))
	if strings.Contains(got, "<script>") {
		t.Errorf("propertyLabelLines must escape HTML-significant characters in an untrusted property value, got %q", got)
	}
}

func TestUnrecognizedARMTypeFallsBackToGenericStyle(t *testing.T) {
	if _, ok := iconForARMType("Microsoft.Something/unknownResource"); ok {
		t.Fatal("test assumption broken: this ARM type must stay unrecognized")
	}
}

func TestRenderInjectionSafety(t *testing.T) {
	doc, g, m := loadFixture(t, azureFixture)
	out := renderXML(t, doc, g, m, fixedTime)
	var v any
	if err := xml.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("output is not well-formed XML: %v", err)
	}
}

func TestRenderCancellation(t *testing.T) {
	doc, g, m := loadFixture(t, azureFixture)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := NewTopologyRenderer(m, g, doc.Metadata, fixedTime)
	var buf bytes.Buffer
	if err := r.Render(ctx, &buf); err == nil {
		t.Fatal("expected cancellation to propagate")
	}
}

func TestRenderPropagatesWriteError(t *testing.T) {
	doc, g, m := loadFixture(t, azureFixture)
	r := NewTopologyRenderer(m, g, doc.Metadata, fixedTime)
	if err := r.Render(context.Background(), failingWriter{}); err == nil {
		t.Fatal("expected the first write error to propagate")
	}
}

type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) { return 0, errWriteFailed }

var errWriteFailed = &testWriteError{}

type testWriteError struct{}

func (*testWriteError) Error() string { return "simulated write failure" }
