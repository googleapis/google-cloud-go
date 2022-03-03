// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package goldmarkcodeblock

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// codeBlockHTMLRenderer is a renderer.NodeRenderer implementation that
// renders CodeBlock nodes.
type codeBlockHTMLRenderer struct {
	html.Config
}

// newCodeBlockHTMLRenderer returns a new CodeblockHTMLRenderer.
func newCodeBlockHTMLRenderer(opts ...html.Option) renderer.NodeRenderer {
	r := &codeBlockHTMLRenderer{
		Config: html.NewConfig(),
	}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

func (r *codeBlockHTMLRenderer) renderCodeBlock(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(`<pre class="prettyprint"><code>`)

		l := n.Lines().Len()
		for i := 0; i < l; i++ {
			line := n.Lines().At(i)
			r.Writer.RawWrite(w, line.Value(source))
		}
	} else {
		_, err := w.WriteString("</code></pre>")
		if err != nil {
			return ast.WalkContinue, err
		}
	}
	return ast.WalkContinue, nil
}

// RegisterFuncs implements renderer.NodeRenderer.RegisterFuncs.
func (r *codeBlockHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, r.renderCodeBlock)
}

type codeBlock struct{}

// CodeBlock is an extenstion to add class="prettyprint" to code blocks.
var CodeBlock = &codeBlock{}

func (c *codeBlock) Extend(m goldmark.Markdown) {
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(newCodeBlockHTMLRenderer(), 1),
	))
}
