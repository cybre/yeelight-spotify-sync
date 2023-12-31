# yeelight-spotify-sync

A little program that uses Spotify track analysis data to create a simple light show using a Yeelight smart color bulb.
It runs in the background on my RaspberryPI 5 polling the Spotify player state and reacts to any changes (start, stop, seek, skip, etc.).
It switches the bulb into music mode on startup to avoid rate limits on the execution of commands.
Also exposes a virtual Homekit device (separate from the actual device Homekit integration) where you can control brightness and power state inside music mode.
