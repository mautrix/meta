use std::ffi::c_void;
use std::panic::{catch_unwind, AssertUnwindSafe};
use std::{ptr, slice};

use generic_array::GenericArray;
use opaque_ke::ciphersuite::CipherSuite;
use opaque_ke::errors::{InternalPakeError, PakeError, ProtocolError};
use opaque_ke::slow_hash::SlowHash;
use opaque_ke::{
    ClientLogin, ClientLoginFinishParameters, ClientLoginStartParameters, CredentialResponse,
};
use rand::rngs::OsRng;
use sha2::Sha512;
use zeroize::{Zeroize, Zeroizing};

struct VestaSuite;
struct VestaPbkdf;

impl SlowHash<Sha512> for VestaPbkdf {
    fn hash(
        mut input: GenericArray<u8, <Sha512 as digest::Digest>::OutputSize>,
    ) -> Result<Vec<u8>, InternalPakeError> {
        let mut output = vec![0; 64];
        pbkdf2::pbkdf2_hmac::<sha2_new::Sha512>(
            &input,
            b"OPAQUE_PBKDF2_SALT",
            100_000,
            &mut output,
        );
        input.zeroize();
        Ok(output)
    }
}

impl CipherSuite for VestaSuite {
    type Group = curve25519_dalek::ristretto::RistrettoPoint;
    type KeyExchange = opaque_ke::key_exchange::tripledh::TripleDH;
    type Hash = Sha512;
    type SlowHash = VestaPbkdf;
}

#[repr(C)]
pub struct LoginStart {
    state: *mut c_void,
    message: [u8; 98],
}

#[repr(C)]
pub struct LoginFinish {
    message: [u8; 64],
    session_key: [u8; 64],
    export_key: [u8; 64],
}

fn status(error: ProtocolError) -> i32 {
    match error {
        ProtocolError::VerificationError(PakeError::InvalidLoginError) => 2,
        _ => 3,
    }
}

fn guarded(run: impl FnOnce() -> Result<(), i32>) -> i32 {
    match catch_unwind(AssertUnwindSafe(run)) {
        Ok(Ok(())) => 0,
        Ok(Err(status)) => status,
        Err(_) => 4,
    }
}

#[no_mangle]
pub unsafe extern "C" fn vesta_opaque_begin(
    password: *const u8,
    password_len: usize,
    output: *mut LoginStart,
) -> i32 {
    if password.is_null() || password_len == 0 || output.is_null() {
        return 1;
    }
    ptr::write(
        output,
        LoginStart {
            state: ptr::null_mut(),
            message: [0; 98],
        },
    );
    guarded(|| {
        let login = ClientLogin::<VestaSuite>::start(
            &mut OsRng,
            slice::from_raw_parts(password, password_len),
            ClientLoginStartParameters::default(),
        )
        .map_err(status)?;
        let message = login
            .message
            .serialize()
            .map_err(status)?
            .try_into()
            .map_err(|_| 3)?;
        ptr::write(
            output,
            LoginStart {
                state: Box::into_raw(Box::new(login.state)).cast(),
                message,
            },
        );
        Ok(())
    })
}

#[no_mangle]
pub unsafe extern "C" fn vesta_opaque_finish(
    state: *mut *mut c_void,
    response: *const u8,
    response_len: usize,
    output: *mut LoginFinish,
) -> i32 {
    if state.is_null() || (*state).is_null() || response.is_null() || output.is_null() {
        return 1;
    }
    ptr::write(
        output,
        LoginFinish {
            message: [0; 64],
            session_key: [0; 64],
            export_key: [0; 64],
        },
    );
    let login =
        *Box::from_raw(ptr::replace(state, ptr::null_mut()).cast::<ClientLogin<VestaSuite>>());
    guarded(|| {
        let response = CredentialResponse::<VestaSuite>::deserialize(slice::from_raw_parts(
            response,
            response_len,
        ))
        .map_err(status)?;
        let result = login
            .finish(response, ClientLoginFinishParameters::default())
            .map_err(status)?;
        let session_key = Zeroizing::new(result.session_key);
        let export_key = Zeroizing::new(result.export_key);
        let message = result.message.serialize().map_err(status)?;
        if message.len() != 64 || session_key.len() != 64 || export_key.len() != 64 {
            return Err(3);
        }
        (*output).message.copy_from_slice(&message);
        (*output).session_key.copy_from_slice(&session_key);
        (*output).export_key.copy_from_slice(export_key.as_slice());
        Ok(())
    })
}

#[no_mangle]
pub unsafe extern "C" fn vesta_opaque_free(state: *mut *mut c_void) {
    if !state.is_null() && !(*state).is_null() {
        drop(Box::from_raw(
            ptr::replace(state, ptr::null_mut()).cast::<ClientLogin<VestaSuite>>(),
        ));
    }
}
