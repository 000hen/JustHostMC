# JustHostMC — Privacy Policy

_Last updated: 2026-06-24_

JustHostMC is a desktop application for creating and managing Minecraft servers
on your own Windows PC. This policy explains what the app does and does not do
with your data.

## Summary

- **Everything runs and stays on your computer.** Servers, worlds, backups, and
  logs are stored locally in the app's data folder.
- **We do not collect, transmit, sell, or share any personal data.**
- **No telemetry or analytics.** The app does not phone home.

## Data the app stores locally

The app and its bundled engine store the following under the app's local data
folder (for the packaged Store build, the per-user package data location, which
Windows removes when you uninstall):

- Server instances and their Minecraft world data.
- Backups you create (portable `.zip` archives).
- Install and console logs (subject to the retention policy you configure).
- A downloaded Java runtime (JRE) and the server software you choose.
- A small settings file and a local server registry.

You can delete all of this at any time from **Settings → Remove all data**, and
it is also removed when you uninstall the app.

## Network access

The app makes outbound HTTPS connections only to download the components you ask
for, from their official sources:

- Minecraft server software (Mojang) and the version manifest.
- PaperMC, MinecraftForge, and NeoForge server software/installers.
- The Eclipse Adoptium (Temurin) Java runtime.

The management connection between the app and its engine is a **loopback-only**
(127.0.0.1) channel authenticated with a per-launch session token; it is never
exposed off the machine.

## Servers you run

When you run a server, it runs **on your PC** and may be reachable by other
players over your network if you forward the port. That is under your control;
the app does not open ports or change firewall/router settings for you.

## Contact

Questions about this policy: hen20090325@gmail.com
