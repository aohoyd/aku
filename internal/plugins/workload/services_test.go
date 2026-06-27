package workload

// targetRef builds a targetRef map for an EndpointSlice address. Empty
// fields are omitted so callers can model absent uid/namespace.
func targetRef(kind, name, namespace, uid string) map[string]any {
	ref := map[string]any{}
	if kind != "" {
		ref["kind"] = kind
	}
	if name != "" {
		ref["name"] = name
	}
	if namespace != "" {
		ref["namespace"] = namespace
	}
	if uid != "" {
		ref["uid"] = uid
	}
	return ref
}
