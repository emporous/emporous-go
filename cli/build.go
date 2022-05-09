package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/uor-framework/client/builder"
	"github.com/uor-framework/client/builder/graph"
	"github.com/uor-framework/client/builder/parser"
	"github.com/uor-framework/client/registryclient/orasclient"
	"github.com/uor-framework/client/util/workspace"
)

type BuildOptions struct {
	*RootOptions
	Destination string
	RootDir     string
	Insecure    bool
	PlainHTTP   bool
	Configs     []string
	Output      string
	Push        bool
}

var clientBuildExamples = templates.Examples(
	`
	# Template content in a directory without pushing
	client build <directory>

	# Template content in a directory and push to a registry location
	client build <directory> --push --destination localhost:5000/myartifacts:latest
	`,
)

func NewBuildCmd(rootOpts *RootOptions) *cobra.Command {
	o := BuildOptions{RootOptions: rootOpts}

	cmd := &cobra.Command{
		Use:           "build directory",
		Short:         "Template, build, and publish OCI content from a local directory",
		Example:       clientBuildExamples,
		SilenceErrors: false,
		SilenceUsage:  false,
		Args:          cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cobra.CheckErr(o.Complete(args))
			cobra.CheckErr(o.Validate())
			cobra.CheckErr(o.Run(cmd.Context()))
		},
	}

	cmd.Flags().StringArrayVarP(&o.Configs, "configs", "c", o.Configs, "auth config paths")
	cmd.Flags().BoolVarP(&o.Insecure, "insecure", "", o.Insecure, "allow connections to SSL registry without certs")
	cmd.Flags().BoolVarP(&o.PlainHTTP, "plain-http", "", o.PlainHTTP, "use plain http and not https")
	cmd.Flags().StringVarP(&o.Output, "output", "o", o.Output, "location to stored templated files")
	cmd.Flags().BoolVarP(&o.Push, "push", "p", o.Push, "push workspace artifacts to registry")
	cmd.Flags().StringVarP(&o.Destination, "destination", "d", o.Destination, "image location to store artifacts in a registry")

	return cmd
}

func (o *BuildOptions) Complete(args []string) error {
	if len(args) < 1 {
		return errors.New("bug: expecting one argument")
	}
	o.RootDir = args[0]
	if o.Output == "" {
		o.Output = "client-workspace"
	}
	return nil
}

func (o *BuildOptions) Validate() error {
	if _, err := os.Stat(o.RootDir); err != nil {
		return fmt.Errorf("workspace directory %q: %v", o.RootDir, err)
	}

	if o.Push && o.Destination == "" {
		return fmt.Errorf("destination must be set when using --push")

	}

	// TODO(jpower432): Validate the reference
	return nil
}

func (o *BuildOptions) Run(ctx context.Context) error {
	_, _ = fmt.Fprintf(o.IOStreams.Out, "Using output directory %q\n", o.Output)
	userSpace, err := workspace.NewLocalWorkspace(o.RootDir)
	if err != nil {
		return err
	}

	g := graph.NewGraph()

	fileIndex := make(map[string]struct{})
	// Do the initial walk to get an index of what is in the workspace
	err = userSpace.Walk(func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("traversing %s: %v", path, err)
		}
		if info == nil {
			return fmt.Errorf("no file info")
		}

		if info.IsDir() {
			return nil
		}

		fileIndex[path] = struct{}{}

		return nil
	})
	if err != nil {
		return err
	}

	// Function to determine whether the
	// data should be replaced in the template
	tFunc := func(value interface{}) bool {
		stringValue, ok := value.(string)
		if !ok {
			return false
		}

		// If the file is found in the workspace
		// return true
		_, found := fileIndex[stringValue]
		return found
	}

	for path := range fileIndex {
		_, _ = fmt.Fprintf(o.IOStreams.Out, "Adding node %s\n", path)
		node := graph.NewNode(path)

		perr := &parser.ErrInvalidFormat{}
		buf := new(bytes.Buffer)
		if err := userSpace.ReadObject(ctx, path, buf); err != nil {
			return err
		}
		p, err := parser.ByContentType(path, buf.Bytes())
		switch {
		case err == nil:
			p.AddFuncs(tFunc)
			node.Template, node.Links, err = p.GetLinkableData(buf.Bytes())
			if err != nil {
				return err
			}
		case !errors.As(err, &perr):
			return err
		}

		g.Nodes[node.Name] = node
	}

	for _, node := range g.Nodes {
		for link, data := range node.Links {
			// Currently with the parsing implementation
			// all initial values are expected to represent
			// the file string data present in the content.
			// FIXME(jpower432): Making this assumption could lead
			// to bug when trying to translate links to a graph.
			fpath, ok := data.(string)
			if !ok {
				return fmt.Errorf("link %q: value should be of type string", link)
			}
			if err := g.AddEdge(node.Name, fpath); err != nil {
				return err
			}
		}
	}

	renderSpace, err := workspace.NewLocalWorkspace(o.Output)
	if err != nil {
		return err
	}

	if err = builder.Build(ctx, g, userSpace, renderSpace); err != nil {
		return fmt.Errorf("error building content: %v", err)
	}

	if o.Push {
		// Gather descriptors written to the render directory for publishing
		client, err := orasclient.NewClient(
			o.Destination,
			orasclient.SkipTLSVerify(o.Insecure),
			orasclient.WithPlainHTTP(o.PlainHTTP),
			orasclient.WithAuthConfigs(o.Configs),
		)
		if err != nil {
			return fmt.Errorf("error configuring client: %v", err)
		}
		var files []string
		err = renderSpace.Walk(func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return fmt.Errorf("traversing %s: %v", path, err)
			}
			if info == nil {
				return fmt.Errorf("no file info")
			}

			if info.Mode().IsRegular() {
				p := renderSpace.Path(path)
				files = append(files, p)
			}
			return nil
		})
		if err != nil {
			return err
		}

		descs, err := client.GatherDescriptors(files...)
		if err != nil {
			return err
		}

		configDesc, err := client.GenerateConfig(nil)
		if err != nil {
			return err
		}

		if err := client.GenerateManifest(configDesc, nil, descs...); err != nil {
			return err
		}

		if err := client.Execute(ctx); err != nil {
			return fmt.Errorf("error publishing content to %s: %v", o.Destination, err)
		}
	}
	return nil
}
