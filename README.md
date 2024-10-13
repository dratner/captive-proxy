# Captive Proxy

Captive Proxy is a service that is meant to run on a software router and detect and proxy
captive portals such as those found in hotels.

For example, if you use a tool like [RaspAP](https://raspap.com/) to implement a travel router
on a Raspberry Pi, you might be frustrated that you need to use VNC or a similar tool to log into
the router and use an on-router browser to deal with a captive portal.

Captive Proxy deals with this by monitoring for captive portals and, when one is detected, 
allowing the next client who connects to the LAN side of the router to deal with the 
captive portal through a proxy.

THIS TOOL IS NOT YET COMPLETE AND IS STILL IN DEVELOPMENT
