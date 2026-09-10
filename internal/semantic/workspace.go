package semantic

import (
	"fmt"
)

func WorkspaceSemanticTarget(number int) string {
	return fmt.Sprintf("workspace-tab-%d", number)
}
