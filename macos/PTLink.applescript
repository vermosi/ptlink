-- PTLink for Mac: a tiny wrapper app.
-- macOS hands ptlink:// links to apps as Apple Events, not as command-line
-- arguments, so this applet receives them and runs the real PTLink program
-- (bundled in Contents/Resources) in a Terminal window you can see.

on run
	launchPTLink("")
end run

on open location theURL
	launchPTLink(theURL)
end open location

on launchPTLink(theURL)
	set bin to POSIX path of (path to me) & "Contents/Resources/ptlink"
	set cmd to "clear; " & quoted form of bin
	if theURL is not "" then set cmd to cmd & " " & quoted form of theURL
	tell application "Terminal"
		activate
		do script cmd
	end tell
end launchPTLink
