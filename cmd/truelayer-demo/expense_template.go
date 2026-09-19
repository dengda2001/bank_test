package main

import "strings"

var expenseTemplate = newWorkspacePageTemplate("expenses-page", nil, strings.Replace(
	embeddedWebText("web/templates/pages/expenses.html"),
	"</head>", "<style>"+workspaceCalendarCSS+"</style><script>"+workspaceCalendarScript+"</script></head>", 1,
))
