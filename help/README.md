### Help file syntax:

## Block begins with @BlockName:

	@PanelNav


## Block header begins after block name with prefix $:

	$This is block One


## Hypertext links syntax: ~linx test~BlockName@:

	~Goto block two~BlockTwo@


## Two blocks with hyperlinks example:

@BlockOne
$This is block one
List of options:
~Go to block two~BlockTwo@

@BlockTwo
$This is block two
List of options:
~Go to block one~BlockOne@


## Variables available:
	%State
	%Ver
	%Platform
	%Backend
	%Host
	%User
	%Admin
	