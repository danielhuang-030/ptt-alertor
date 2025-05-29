package myutil

import (
	"errors"
	"strings"
)

const discordIDSeparator = "_CH#_"

// CreateDiscordInternalID creates a combined internal ID from a Discord User ID and Channel ID.
// The format is userID + "_CH#_" + channelID.
func CreateDiscordInternalID(userID, channelID string) string {
	return userID + discordIDSeparator + channelID
}

// ParseDiscordInternalID parses an internal Discord ID string into its constituent User ID and Channel ID.
// It returns an error if the internalID format is invalid.
func ParseDiscordInternalID(internalID string) (userID, channelID string, err error) {
	parts := strings.SplitN(internalID, discordIDSeparator, 2)
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	}
	return "", "", errors.New("invalid internal Discord ID format")
}
