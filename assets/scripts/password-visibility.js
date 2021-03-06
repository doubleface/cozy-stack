(function (d) {
  var passwordVisibility = false
  var passwordInput = d.getElementById('password')
  var passwordVisibilityButton = d.getElementById('password-visibility-button')
  passwordVisibilityButton.addEventListener('click', function (event) {
    event.preventDefault()
    passwordVisibility = !passwordVisibility
    passwordInput.type = passwordVisibility ? 'text' : 'password'
    passwordVisibilityButton.setAttribute('aria-pressed', passwordVisibility)
  })
})(window.document)
