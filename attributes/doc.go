/*
Copyright 2022 Emporous Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package attributes

// This package defines basic concrete implementation for
// model.Attribute kinds and model.AttributeSets.

// This package contains helper functions when creating new attributes.
// WARNING: The helper functions (attribute.Reflect) in this package use reflection and therefore will incur a performance penalty.

// Similar implementation to the go-ipld-prime library.
// Reference: https://github.com/ipld/go-ipld-prime/tree/master/node/basicnode
