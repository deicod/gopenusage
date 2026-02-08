package builtin

import (
	"github.com/deicod/gopenusage/pkg/openusage"
	"github.com/deicod/gopenusage/pkg/openusage/plugins/antigravity"
	"github.com/deicod/gopenusage/pkg/openusage/plugins/claude"
	"github.com/deicod/gopenusage/pkg/openusage/plugins/codex"
	"github.com/deicod/gopenusage/pkg/openusage/plugins/copilot"
	"github.com/deicod/gopenusage/pkg/openusage/plugins/cursor"
	"github.com/deicod/gopenusage/pkg/openusage/plugins/mock"
	"github.com/deicod/gopenusage/pkg/openusage/plugins/windsurf"
)

func Plugins() []openusage.Plugin {
	return []openusage.Plugin{
		antigravity.New(),
		claude.New(),
		codex.New(),
		copilot.New(),
		cursor.New(),
		mock.New(),
		windsurf.New(),
	}
}
