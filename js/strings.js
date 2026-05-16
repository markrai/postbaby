const rainbowColors = ['#FFFFFF', '#DC143C', '#FF7F00', '#FFBF00', '#AAFF00', '#89CFF0', '#CBC3E3', '#CF9FFF']; // Red, Orange, Yellow, Green, Blue, Purple, Violet

const strings = {
    // Mobile-specific notes
    defaultNotesMobile: [
        'long-press a note to delete it!',
        'double-tap to edit notes!',
        'long-press on canvas to create new note!',
        'tap a note to change its color'
    ],
    positionsMobile: [                  // MOBILE 📱
        { top: '-660.73px', left: '-188.625px' },  // 'long-press a note to delete it!',
        {top: '-670.616px', left: '83.9469px'}, // 'double-tap to edit notes!',
        {top: '-304.169px', left: '77.0204px'},     // 'long-press on canvas to create new note!',
        {top: '-274.074px', left: '-187.654px'} // 'tap a note to change its color'
        
    ],
    colorsMobile: [
        '#FFCCCC', // Light red
        '#CCFFCC', // Light green
        '#CCCCFF',  // Light blue
        '#FFBF00'  // Amber
    ],

    // Desktop-specific notes in landscape mode
    defaultNotesDesktopLandscape: [
        'Right-click on canvas to create a new note!',
        'Left-Click on this note to change its color',
        'Double-click to edit notes!',
        'Press c to clear all notes',
        'Right click on this note to delete it\n or drag to toilet-roll',
        'Ask us about creating a customized postbaby for you!'
    ],
    positionsDesktopLandscape: [         // DESKTOP LANDSCAPE 🌆
        { top: '-90%', left: '1000%' }, // 'Right-click on canvas to create a new note!',
        { top: '-418px', left: '-529px' }, // 'Left-click on a note to change its color
        { top: '-90%', left: '-900%' }, // 'Double-click to edit notes!',
        { top: '-60%', left: '1400%' }, // 'Press c to clear all notes',
        { top: '-315px', left: '-450px' }, // 'Right click on this note to delete it\n or drag to toilet-roll',
        { top: '-10%', left: '40%' } // 'Ask us about creating a customized postbaby for you!'
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
        'Ask us about creating a customized postbaby for you!'
    ],
    positionsDesktopPortrait: [              // DESKTOP PORTRAIT 🖼️
        { top: '-45%', left: '-900%' },  // 'Right-click on canvas\n to create a new note!',
        { top: '-45%', left: '500%' },   // 'Double-click to edit notes!',
        { top: '-30%', left: '-1500%' }, // 'Press c to clear all notes',
        { top: '-30%', left: '700%' }, // 'Right click on this note to delete it!',
        { top: '-5%', left: '-5%' }, // 'Ask us about creating a\n customized postbaby for you!'
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
    confirmDeleteAllItemsText: "Are you sure you want to delete ALL items?",
    confirmDeleteEdgeText: "Are you sure you want to delete this connection?"
};

