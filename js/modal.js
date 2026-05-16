function setConfirmMessage(text) {
    document.querySelector('#confirmModal .modal-content p').innerText = text;
}

function showConfirmModal() {
    const confirmModal = document.getElementById('confirmModal');
    const confirmDeleteButton = document.getElementById('confirmDelete');
    if (confirmModal && confirmDeleteButton) {
        confirmModal.style.display = 'flex';
        confirmDeleteButton.focus(); // Focus on the "Yes" button
    } else {
        console.error('Confirm modal elements not found.');
    }
}
