const strings = {
    // Mobile-specific notes
    defaultNotesMobile: [
        'long-press a note to delete it!',
        'double-tap to edit notes!',
        'long-press on canvas to create new note!',
        'tap a note to change its color'
    ],
    positionsMobile: [
        { top: '-420px', left: '0%' },
        { top: '-350px', left: '80%' },
        { top: '0px', left: '0%' },
        { top: '-200px', left: '80%' }
        
    ],
    colorsMobile: [
        '#FFCCCC', // Light red
        '#CCFFCC', // Light green
        '#CCCCFF',  // Light blue
        '#FFBF00'  // Amber
    ],

    // Desktop-specific notes in landscape mode
    defaultNotesDesktopLandscape: [
        'Right-click on canvas\n to create a new note!',
        'Double-click to edit notes!',
        'Press c to clear all notes',
        'Right click on this note to delete it\n or drag to toilet-roll',
        'Ask us about creating a\n customized postbaby for you!\n\nmarkraidc@gmail.com'
    ],
    positionsDesktopLandscape: [
        { top: '-480%', left: '10%' },
        { top: '-200%', left: '70%' },
        { top: '-200%', left: '10%' },
        { top: '-400%', left: '70%' },
        { top: '-60%', left: '40%' }
    ],
    colorsDesktopLandscape: [
        '#FFD700', // Gold
        '#ADFF2F', // GreenYellow
        '#1E90FF', // DodgerBlue
        '#FF69B4', // HotPink
        '#FF8C00'  // DarkOrange
    ],

    // Desktop-specific notes in portrait mode
    defaultNotesDesktopPortrait: [
        'Right-click on canvas\n to create a new note!',
        'Double-click to edit notes!',
        'Press c to clear all notes',
        'Right click on this note to delete it!',
        'Ask us about creating a\n customized postbaby for you!'
    ],
    positionsDesktopPortrait: [
        { top: '-100%', left: '10%' },
        { top: '-90%', left: '60%' },
        { top: '-30%', left: '-5%' },
        { top: '-50%', left: '75%' },
        { top: '3%', left: '35%' }
    ],
    colorsDesktopPortrait: [
        '#FFA07A', // LightSalmon
        '#98FB98', // PaleGreen
        '#87CEFA', // LightSkyBlue
        '#DDA0DD', // Plum
        '#F4A460'  // SandyBrown
    ],

    // Confirmation modal texts
    confirmDeleteItemText: "Are you sure you want to delete this item?",
    confirmDeleteAllItemsText: "Are you sure you want to delete ALL items?"
};
