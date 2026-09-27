package graphqlapi

import "testing"

func TestGraphQLSelectedMutationUsesParsedOperation(t *testing.T) {
	t.Parallel()
	if graphQLSelectedMutation("query { __typename } mutation Hidden { __typename }", "") {
		t.Fatal("an unselected mutation must not count")
	}
	if !graphQLSelectedMutation("query Read { __typename } mutation Write { __typename }", "Write") {
		t.Fatal("named mutation was not selected")
	}
	if graphQLSelectedMutation("# comment\nquery { __typename }", "") {
		t.Fatal("a query was treated as a mutation")
	}
	if !graphQLSelectedMutation("mutation { __typename }", "") {
		t.Fatal("anonymous mutation was missed")
	}
}
