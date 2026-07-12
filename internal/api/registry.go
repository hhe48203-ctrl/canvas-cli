package api

type Operation struct {
	ID          string       `json:"id" yaml:"id"`
	Method      string       `json:"method" yaml:"method"`
	Path        string       `json:"path" yaml:"path"`
	Summary     string       `json:"summary" yaml:"summary"`
	Description string       `json:"description,omitempty" yaml:"description,omitempty"`
	Group       string       `json:"group,omitempty" yaml:"group,omitempty"`
	DocsURL     string       `json:"docs_url,omitempty" yaml:"docs_url,omitempty"`
	Scopes      []string     `json:"scopes,omitempty" yaml:"scopes,omitempty"`
	Parameters  []Parameter  `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	RequestBody *RequestBody `json:"request_body,omitempty" yaml:"request_body,omitempty"`
	Responses   []Response   `json:"responses,omitempty" yaml:"responses,omitempty"`
	// PathParams and QueryParams retain compatibility with older generated catalogs.
	PathParams  []string `json:"path_params,omitempty" yaml:"path_params,omitempty"`
	QueryParams []string `json:"query_params,omitempty" yaml:"query_params,omitempty"`
}

type Parameter struct {
	Name        string   `json:"name" yaml:"name"`
	In          string   `json:"in" yaml:"in"`
	Required    bool     `json:"required" yaml:"required"`
	Type        string   `json:"type,omitempty" yaml:"type,omitempty"`
	Format      string   `json:"format,omitempty" yaml:"format,omitempty"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`
	Enum        []string `json:"enum,omitempty" yaml:"enum,omitempty"`
	Default     string   `json:"default,omitempty" yaml:"default,omitempty"`
}

type RequestBody struct {
	Required     bool     `json:"required" yaml:"required"`
	Description  string   `json:"description,omitempty" yaml:"description,omitempty"`
	ContentTypes []string `json:"content_types,omitempty" yaml:"content_types,omitempty"`
}

type Response struct {
	StatusCode   string   `json:"status_code" yaml:"status_code"`
	Description  string   `json:"description,omitempty" yaml:"description,omitempty"`
	ContentTypes []string `json:"content_types,omitempty" yaml:"content_types,omitempty"`
}

var FallbackOperations = []Operation{
	{ID: "me", Method: "GET", Path: "/api/v1/users/self", Summary: "Get the current user"},
	{ID: "courses.list", Method: "GET", Path: "/api/v1/courses", Summary: "List the current user's courses", Parameters: []Parameter{{Name: "enrollment_type", In: "query", Type: "string"}, {Name: "include[]", In: "query", Type: "array"}}},
	{ID: "courses.show", Method: "GET", Path: "/api/v1/courses/{course_id}", Summary: "Get a course", Parameters: []Parameter{{Name: "course_id", In: "path", Required: true, Type: "string"}}},
	{ID: "assignments.list", Method: "GET", Path: "/api/v1/courses/{course_id}/assignments", Summary: "List assignments", PathParams: []string{"course_id"}, QueryParams: []string{"include[]", "order_by", "bucket"}},
	{ID: "assignments.show", Method: "GET", Path: "/api/v1/courses/{course_id}/assignments/{assignment_id}", Summary: "Get an assignment", PathParams: []string{"course_id", "assignment_id"}},
	{ID: "calendar.list", Method: "GET", Path: "/api/v1/calendar_events", Summary: "List calendar events", QueryParams: []string{"context_codes[]", "start_date", "end_date"}},
	{ID: "files.list", Method: "GET", Path: "/api/v1/courses/{course_id}/files", Summary: "List course files", PathParams: []string{"course_id"}},
	{ID: "submissions.list", Method: "GET", Path: "/api/v1/courses/{course_id}/students/submissions", Summary: "List submissions", PathParams: []string{"course_id"}, QueryParams: []string{"student_ids[]", "assignment_ids[]"}},
	{ID: "quizzes.list", Method: "GET", Path: "/api/v1/courses/{course_id}/quizzes", Summary: "List classic quizzes", PathParams: []string{"course_id"}},
	{ID: "quizzes.start", Method: "POST", Path: "/api/v1/courses/{course_id}/quizzes/{quiz_id}/submissions", Summary: "Start a quiz submission", PathParams: []string{"course_id", "quiz_id"}},
	{ID: "quiz-submissions.questions", Method: "GET", Path: "/api/v1/quiz_submissions/{quiz_submission_id}/questions", Summary: "List quiz submission questions", PathParams: []string{"quiz_submission_id"}},
}

func (op Operation) ParametersIn(location string) []Parameter {
	var result []Parameter
	for _, parameter := range op.Parameters {
		if parameter.In == location {
			result = append(result, parameter)
		}
	}
	if len(result) == 0 {
		var names []string
		if location == "path" {
			names = op.PathParams
		} else if location == "query" {
			names = op.QueryParams
		}
		for _, name := range names {
			result = append(result, Parameter{Name: name, In: location, Required: location == "path"})
		}
	}
	return result
}

// Operations combines generated OpenAPI metadata with stable high-level aliases.
// The fallback registry keeps the CLI useful before generation is run.
var Operations = buildOperations()

func buildOperations() []Operation {
	result := append([]Operation(nil), GeneratedOperations...)
	seen := map[string]bool{}
	for _, op := range result {
		seen[op.ID] = true
	}
	for _, op := range FallbackOperations {
		if !seen[op.ID] {
			result = append(result, op)
		}
	}
	return result
}

func Find(id string) (Operation, bool) {
	for _, op := range Operations {
		if op.ID == id {
			return op, true
		}
	}
	return Operation{}, false
}
