// This file colors the confirmed-panel port markers by a
// network.router.port resource's own resource-level Status. Unlike
// Cisco NX-OS's own binary active/inactive marker (that producer's
// real fixtures never carried a meaningfully different third state),
// this producer's own real data carries "degraded" as a genuinely
// distinct, common state (a port that is administratively down or
// otherwise not passing traffic, distinct from a fully inactive one),
// so it gets its own color rather than folding into the neutral
// fallback.
package vmanage

// portMarkerStyleForStatus returns the confirmed-panel marker color for
// a network.router.port resource's own Status: green for "active",
// amber for "degraded", red for "inactive", and the neutral
// portMarkerStyle for anything else ("retired", "unknown", or absent)
// an unrecognized status is left neutral rather than guessed either way
func portMarkerStyleForStatus(status string) string {
	switch status {
	case "active":
		return portMarkerActiveStyle
	case "degraded":
		return portMarkerDegradedStyle
	case "inactive":
		return portMarkerInactiveStyle
	default:
		return portMarkerStyle
	}
}

const (
	portMarkerActiveStyle   = "ellipse;whiteSpace=wrap;html=1;fillColor=#d5e8d4;strokeColor=#82b366;strokeWidth=2;"
	portMarkerDegradedStyle = "ellipse;whiteSpace=wrap;html=1;fillColor=#ffe6cc;strokeColor=#d79b00;strokeWidth=2;"
	portMarkerInactiveStyle = "ellipse;whiteSpace=wrap;html=1;fillColor=#f8cecc;strokeColor=#b85450;strokeWidth=2;"
	portMarkerStyle         = "ellipse;whiteSpace=wrap;html=1;fillColor=#ff8000;strokeColor=#802900;strokeWidth=2;"
)

// portContainerStyleForStatus returns a port's own container (swimlane)
// style, colored exactly the same as portMarkerStyleForStatus's own
// marker for that same status, one consistent status-color mapping
// applied to both, rather than a second, differently-keyed palette for
// the container.
func portContainerStyleForStatus(status string) string {
	switch status {
	case "active":
		return portContainerActiveStyle
	case "degraded":
		return portContainerDegradedStyle
	case "inactive":
		return portContainerInactiveStyle
	default:
		return portContainerStyle
	}
}

const (
	portContainerActiveStyle   = "swimlane;whiteSpace=wrap;html=1;fillColor=#d5e8d4;strokeColor=#82b366;rounded=1;collapsible=0;fontSize=9;startSize=20;"
	portContainerDegradedStyle = "swimlane;whiteSpace=wrap;html=1;fillColor=#ffe6cc;strokeColor=#d79b00;rounded=1;collapsible=0;fontSize=9;startSize=20;"
	portContainerInactiveStyle = "swimlane;whiteSpace=wrap;html=1;fillColor=#f8cecc;strokeColor=#b85450;rounded=1;collapsible=0;fontSize=9;startSize=20;"
	portContainerStyle         = "swimlane;whiteSpace=wrap;html=1;fillColor=#ff8000;strokeColor=#802900;rounded=1;collapsible=0;fontSize=9;startSize=20;"
)
